package order

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	orderhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/order"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

func newHistoryFixture(t *testing.T) *creationFixture {
	t.Helper()
	f := newCreationFixture(t)
	auth, err := authservice.NewService(authrepository.NewRepository(f.pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	history := NewHistoryService(orderrepository.NewRepository(), repository.NewRepository(f.pool, zap.NewNop()))
	basehandler.RegisterOrderHistoryRoutes(f.router.(*gin.Engine).Group("/api/v1"), orderhandler.NewHistoryHandler(history), authhandler.NewHandler(auth))
	return f
}

func historyRequest(f *creationFixture, path string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil).WithContext(f.ctx)
	if authenticated {
		request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}

func createHistoryOrder(t *testing.T, f *creationFixture, items ...dto.OrderRequestItem) dto.OrderDetail {
	t.Helper()
	response := f.createRequest(orderBody(t, items...), uuid.New())
	if response.Code != http.StatusCreated {
		t.Fatalf("create history order: %d %s", response.Code, response.Body.String())
	}
	var result dto.OrderDetail
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOrderHistoryIntegrationList(t *testing.T) {
	f := newHistoryFixture(t)
	product := f.product(t, "Ingredient")
	f.batch(t, product, "100.000", f.now.Add(time.Hour))
	first, second := f.dish(t, `Pizza%_\`, true), f.dish(t, "Other", true)
	f.recipe(t, first, 1, map[uuid.UUID]string{product: "1.000"})
	f.recipe(t, second, 1, map[uuid.UUID]string{product: "1.000"})
	one := createHistoryOrder(t, f, dto.OrderRequestItem{DishID: first.String(), Portions: 2}, dto.OrderRequestItem{DishID: second.String(), Portions: 1})
	two := createHistoryOrder(t, f, dto.OrderRequestItem{DishID: second.String(), Portions: 1})
	f.exec(t, `UPDATE dishes SET name='Renamed',is_active=false WHERE id=$1`, first)
	f.recipe(t, first, 2, map[uuid.UUID]string{product: "2.000"})
	before := f.snapshot(t)
	ids := []uuid.UUID{one.ID, two.ID}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return -slices.Compare(a[:], b[:]) })
	for _, scenario := range []struct {
		name  string
		query url.Values
		total int64
		ids   []uuid.UUID
	}{
		{"all", nil, 2, ids},
		{"first page", url.Values{"page_size": {"1"}}, 2, ids[:1]},
		{"second page", url.Values{"page": {"2"}, "page_size": {"1"}}, 2, ids[1:]},
		{"outside page", url.Values{"page": {"3"}, "page_size": {"1"}}, 2, []uuid.UUID{}},
		{"literal snapshot", url.Values{"search": {`pIZZA%_\`}}, 1, []uuid.UUID{one.ID}},
		{"wildcard percent", url.Values{"search": {"%"}}, 1, []uuid.UUID{one.ID}},
		{"wildcard underscore", url.Values{"search": {"_"}}, 1, []uuid.UUID{one.ID}},
		{"new name excluded", url.Values{"search": {"Renamed"}}, 0, []uuid.UUID{}},
		{"multiple matching items do not duplicate", url.Values{"search": {"o"}}, 2, ids},
		{"number", url.Values{"search": {one.Number}}, 1, []uuid.UUID{one.ID}},
		{"empty search", url.Values{"search": {""}}, 2, ids},
		{"SQL literal", url.Values{"search": {"' OR true --"}}, 0, []uuid.UUID{}},
		{"inclusive from", url.Values{"from": {f.now.Format(time.RFC3339Nano)}}, 2, ids},
		{"exclusive to", url.Values{"to": {f.now.Format(time.RFC3339Nano)}}, 0, []uuid.UUID{}},
		{"nanosecond from", url.Values{"from": {f.now.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, 0, []uuid.UUID{}},
		{"nanosecond to", url.Values{"to": {f.now.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, 2, ids},
		{"combined", url.Values{"search": {`Pizza%_\`}, "from": {f.now.Format(time.RFC3339Nano)}, "to": {f.now.Add(time.Microsecond).Format(time.RFC3339Nano)}}, 1, []uuid.UUID{one.ID}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			response := historyRequest(f, "/orders?"+scenario.query.Encode(), true)
			var result dto.ListResponse[dto.OrderSummary]
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
				t.Fatalf("list: %d %s", response.Code, response.Body.String())
			}
			got := make([]uuid.UUID, 0, len(result.Items))
			for _, order := range result.Items {
				got = append(got, order.ID)
				if order.Status != "completed" || order.CreatedBy.ID != f.actor || !order.CreatedAt.Equal(f.now) || order.Items == nil {
					t.Fatalf("bad summary: %+v", order)
				}
				wantPortions := 1
				if order.ID == one.ID {
					wantPortions = 3
				}
				if order.TotalPortions != wantPortions {
					t.Fatalf("total portions: %d want %d", order.TotalPortions, wantPortions)
				}
				if order.ID == one.ID {
					for _, item := range order.Items {
						if item.DishID == first && (item.DishName != `Pizza%_\` || item.Portions != 2 || item.RecipeVersion == nil || *item.RecipeVersion != 1) {
							t.Fatalf("changed historical item: %+v", item)
						}
					}
				}
			}
			if result.Total != scenario.total || !reflect.DeepEqual(got, scenario.ids) || result.Items == nil || result.Meta == nil {
				t.Fatalf("list total/order/shape: %+v want %v", result, scenario.ids)
			}
		})
	}
	if f.snapshot(t) != before {
		t.Fatal("history reads wrote stock or order facts")
	}
}

func TestOrderHistoryIntegrationListAccessAndValidation(t *testing.T) {
	f := newHistoryFixture(t)
	for _, role := range []string{"admin", "manager", "order_operator", "storekeeper"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		want := http.StatusOK
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		response := historyRequest(f, "/orders", true)
		if response.Code != want {
			t.Fatalf("%s access: %d %s", role, response.Code, response.Body.String())
		}
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	for _, query := range []string{"from=2026-10-10", "page_size=101", "search=a&search=b", "unknown=1", "sort=created_at", "from=2026-10-11T00:00:00Z&to=2026-10-10T00:00:00Z"} {
		if response := historyRequest(f, "/orders?"+query, true); response.Code != 422 || !strings.Contains(response.Body.String(), "VALIDATION_ERROR") {
			t.Fatalf("validation: %d %s", response.Code, response.Body.String())
		}
	}
	if response := historyRequest(f, "/orders?unknown=1", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %s", response.Code, response.Body.String())
	}
	f.exec(t, `UPDATE sessions SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour'`)
	if response := historyRequest(f, "/orders", true); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: %d %s", response.Code, response.Body.String())
	}
}
