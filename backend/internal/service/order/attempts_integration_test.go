package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func createHistoryAttempt(t *testing.T, f *creationFixture, key uuid.UUID, items ...dto.OrderRequestItem) uuid.UUID {
	t.Helper()
	body := orderBody(t, items...)
	response := f.createRequest(body, key)
	if response.Code != http.StatusConflict {
		t.Fatalf("create history attempt: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Error struct {
			Details struct {
				AttemptID uuid.UUID `json:"attempt_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Error.Details.AttemptID == uuid.Nil {
		t.Fatalf("attempt ID: %v %s", err, response.Body.String())
	}
	replay := f.createRequest(body, key)
	if replay.Code != response.Code || replay.Body.String() != response.Body.String() {
		t.Fatal("attempt replay changed")
	}
	return result.Error.Details.AttemptID
}

func TestOrderHistoryIntegrationAttempts(t *testing.T) {
	f := newHistoryFixture(t)
	product := f.product(t, "Ingredient")
	f.batch(t, product, "5.000", f.now.Add(time.Hour))
	dish, second, unavailable := f.dish(t, `Snapshot%_\ Pizza`, true), f.dish(t, "Other snapshot", true), f.dish(t, "Unavailable", false)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	f.recipe(t, second, 1, map[uuid.UUID]string{product: "1.000"})
	createHistoryOrder(t, f, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	shortage := createHistoryAttempt(t, f, uuid.New(), dto.OrderRequestItem{DishID: dish.String(), Portions: 50}, dto.OrderRequestItem{DishID: second.String(), Portions: 1})
	version := 2
	changed := createHistoryAttempt(t, f, uuid.New(), dto.OrderRequestItem{DishID: dish.String(), Portions: 1, RecipeVersion: &version})
	disabled := createHistoryAttempt(t, f, uuid.New(), dto.OrderRequestItem{DishID: unavailable.String(), Portions: 1})
	f.exec(t, `UPDATE dishes SET name=name||' renamed',is_active=false`)
	f.recipe(t, dish, 3, map[uuid.UUID]string{product: "2.000"})
	f.exec(t, `UPDATE products SET name='Renamed ingredient' WHERE id=$1`, product)
	ids := []uuid.UUID{shortage, changed, disabled}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return -slices.Compare(a[:], b[:]) })
	matching := slices.DeleteFunc(slices.Clone(ids), func(id uuid.UUID) bool { return id == disabled })
	before := f.snapshot(t)
	for _, scenario := range []struct {
		name  string
		query url.Values
		total int64
		ids   []uuid.UUID
	}{
		{"all", nil, 3, ids},
		{"saved dish", url.Values{"search": {`sNaPsHoT%_\`}}, 2, matching},
		{"no duplicate multi-item attempt", url.Values{"search": {"snapshot"}}, 2, matching},
		{"literal percent", url.Values{"search": {"%"}}, 2, matching},
		{"literal underscore", url.Values{"search": {"_"}}, 2, matching},
		{"new name excluded", url.Values{"search": {"renamed"}}, 0, []uuid.UUID{}},
		{"ID", url.Values{"search": {strings.ToUpper(shortage.String())}}, 1, []uuid.UUID{shortage}},
		{"ID part", url.Values{"search": {shortage.String()[:8]}}, 1, []uuid.UUID{shortage}},
		{"empty search", url.Values{"search": {""}}, 3, ids},
		{"SQL literal", url.Values{"search": {"' OR true --"}}, 0, []uuid.UUID{}},
		{"shortage reason", url.Values{"reason_code": {"INSUFFICIENT_STOCK"}}, 1, []uuid.UUID{shortage}},
		{"changed reason", url.Values{"reason_code": {"RECIPE_CHANGED"}}, 1, []uuid.UUID{changed}},
		{"unavailable reason", url.Values{"reason_code": {"DISH_UNAVAILABLE"}}, 1, []uuid.UUID{disabled}},
		{"first page", url.Values{"page_size": {"1"}}, 3, ids[:1]},
		{"second page", url.Values{"page": {"2"}, "page_size": {"1"}}, 3, ids[1:2]},
		{"past end", url.Values{"page": {"4"}, "page_size": {"1"}}, 3, []uuid.UUID{}},
		{"inclusive from", url.Values{"from": {f.now.Format(time.RFC3339Nano)}}, 3, ids},
		{"exclusive to", url.Values{"to": {f.now.Format(time.RFC3339Nano)}}, 0, []uuid.UUID{}},
		{"from submicrosecond", url.Values{"from": {f.now.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, 0, []uuid.UUID{}},
		{"to submicrosecond", url.Values{"to": {f.now.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, 3, ids},
		{"combined", url.Values{"search": {"snapshot"}, "reason_code": {"RECIPE_CHANGED"}, "from": {f.now.Format(time.RFC3339Nano)}, "to": {f.now.Add(time.Microsecond).Format(time.RFC3339Nano)}}, 1, []uuid.UUID{changed}},
		{"empty intersection", url.Values{"search": {"Unavailable"}, "reason_code": {"RECIPE_CHANGED"}}, 0, []uuid.UUID{}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			response := historyRequest(f, "/order-attempts?"+scenario.query.Encode(), true)
			var result dto.ListResponse[dto.OrderAttempt]
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
				t.Fatalf("attempts: %d %s", response.Code, response.Body.String())
			}
			got := make([]uuid.UUID, 0, len(result.Items))
			for _, attempt := range result.Items {
				got = append(got, attempt.ID)
				if attempt.CreatedBy.ID != f.actor || attempt.CreatedBy.Name != "Operator" || !attempt.CreatedAt.Equal(f.now) || attempt.Items == nil || attempt.Shortages == nil || attempt.Issues == nil {
					t.Fatalf("bad attempt projection: %+v", attempt)
				}
				position := slices.IndexFunc(attempt.Items, func(item dto.OrderItemSnapshot) bool { return item.DishID == dish })
				if attempt.ID == shortage && (attempt.ReasonCode != "INSUFFICIENT_STOCK" || len(attempt.Items) != 2 || position < 0 || attempt.Items[position].DishName != `Snapshot%_\ Pizza` || attempt.Items[position].Portions != 50 || len(attempt.Shortages) != 1 || attempt.Shortages[0].ProductName != "Ingredient" || attempt.Shortages[0].MissingQuantity != "47.000") {
					t.Fatalf("changed shortage snapshot: %+v", attempt)
				}
				if attempt.ID == changed && (attempt.ReasonCode != "RECIPE_CHANGED" || len(attempt.Issues) != 1 || attempt.Issues[0].CurrentRecipeVersion == nil || *attempt.Issues[0].CurrentRecipeVersion != 1) {
					t.Fatalf("changed issue snapshot: %+v", attempt)
				}
				if attempt.ID == disabled && (attempt.ReasonCode != "DISH_UNAVAILABLE" || attempt.Items[0].RecipeVersion != nil) {
					t.Fatalf("unavailable snapshot: %+v", attempt)
				}
			}
			if result.Total != scenario.total || !reflect.DeepEqual(got, scenario.ids) || result.Items == nil || result.Meta == nil {
				t.Fatalf("attempt total/order/shape: %+v want %v", result, scenario.ids)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			var rawItems []map[string]json.RawMessage
			if err := json.Unmarshal(raw["items"], &rawItems); err != nil {
				t.Fatal(err)
			}
			for _, item := range rawItems {
				for _, forbidden := range []string{"number", "order_id", "status"} {
					if _, exists := item[forbidden]; exists {
						t.Fatalf("attempt exposes %s", forbidden)
					}
				}
			}
		})
	}
	if f.snapshot(t) != before || f.count(t, "orders") != 1 || f.count(t, "order_attempts") != 3 {
		t.Fatal("history read or idempotent replay created extra facts")
	}
}

type historyBrokenBody struct{}

func (historyBrokenBody) Read([]byte) (int, error) { return 0, errors.New("test transport failure") }
func (historyBrokenBody) Close() error             { return nil }

func TestOrderHistoryIntegrationExcludesEarlyAndTechnicalFailures(t *testing.T) {
	f := newHistoryFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	if f.check(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1}).CanCreate {
		t.Fatal("fixture needs a refusal")
	}
	for _, invalid := range []string{"{", `{"items":[]}`, orderBody(t, dto.OrderRequestItem{DishID: uuid.NewString(), Portions: 1})} {
		if response := f.createRequest(invalid, uuid.New()); response.Code != 400 && response.Code != 422 {
			t.Fatalf("invalid request: %d", response.Code)
		}
	}
	f.exec(t, `UPDATE users SET role='storekeeper' WHERE id=$1`, f.actor)
	if response := f.createRequest(body, uuid.New()); response.Code != http.StatusForbidden {
		t.Fatal("access failure expected")
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil).WithContext(f.ctx)
	request.Body = historyBrokenBody{}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://flagman.localhost")
	request.Header.Set("X-CSRF-Token", "csrf")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("transport: %d %s", response.Code, response.Body.String())
	}
	f.exec(t, `ALTER TABLE idempotency_keys ADD CONSTRAINT history_reject_write CHECK(false) NOT VALID`)
	if response := f.createRequest(body, uuid.New()); response.Code != http.StatusInternalServerError {
		t.Fatalf("technical refusal: %d", response.Code)
	}
	f.exec(t, `ALTER TABLE idempotency_keys DROP CONSTRAINT history_reject_write`)
	response = historyRequest(f, "/order-attempts", true)
	var result dto.ListResponse[dto.OrderAttempt]
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Total != 0 || result.Items == nil || len(result.Items) != 0 || f.count(t, "idempotency_keys") != 0 {
		t.Fatalf("non-domain failures entered history: %d %s", response.Code, response.Body.String())
	}
}

func TestOrderHistoryIntegrationAttemptsAccessAndValidation(t *testing.T) {
	f := newHistoryFixture(t)
	for _, role := range []string{"admin", "manager", "order_operator", "storekeeper"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		want := http.StatusOK
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		if response := historyRequest(f, "/order-attempts", true); response.Code != want {
			t.Fatalf("%s access: %d", role, response.Code)
		}
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	for _, query := range []string{"reason_code=", "reason_code=ingredient_shortage", "reason_code=TRANSPORT_ERROR", "reason_code=RECIPE_CHANGED&reason_code=DISH_UNAVAILABLE", "from=2026-10-10", "page_size=101", "search=a&search=b", "unknown=1", "sort=created_at", "from=2026-10-11T00:00:00Z&to=2026-10-10T00:00:00Z"} {
		if response := historyRequest(f, "/order-attempts?"+query, true); response.Code != 422 {
			t.Fatalf("%s validation: %d %s", query, response.Code, response.Body.String())
		}
	}
	if response := historyRequest(f, "/order-attempts?unknown=1", false); response.Code != http.StatusUnauthorized {
		t.Fatal("authentication must precede validation")
	}
}
