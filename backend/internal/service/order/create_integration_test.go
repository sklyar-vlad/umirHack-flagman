package order

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"go.uber.org/zap"
)

type creationFixture struct {
	*orderFixture
	create *CreateService
}

func newCreationFixture(t *testing.T) *creationFixture {
	t.Helper()
	f := newOrderFixture(t)
	base := repository.NewRepository(f.pool, zap.NewNop())
	create := NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	create.now = func() time.Time { return f.now }
	auth, err := authservice.NewService(authrepository.NewRepository(f.pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	basehandler.RegisterOrderCreateRoutes(
		f.router.(*gin.Engine).Group("/api/v1"),
		orderhandler.NewCreateHandler(create),
		authhandler.NewHandler(auth),
	)
	return &creationFixture{f, create}
}

func (f *creationFixture) createRequest(body string, key uuid.UUID) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	request = request.WithContext(f.ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://flagman.localhost")
	request.Header.Set("X-CSRF-Token", "csrf")
	request.Header.Set("Idempotency-Key", key.String())
	request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}

func orderBody(t *testing.T, items ...dto.OrderRequestItem) string {
	t.Helper()
	body, err := json.Marshal(dto.OrderRequest{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func (f *creationFixture) count(t *testing.T, table string) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestOrderCreateIntegrationPersistsFullFEFOOrder(t *testing.T) {
	f := newCreationFixture(t)
	x, y, z := f.product(t, "Shared"), f.product(t, "Other first"), f.product(t, "Other second")
	f.batch(t, x, "0.400", f.now.Add(time.Hour))
	f.batch(t, x, "0.400", f.now.Add(2*time.Hour))
	f.batch(t, x, "10.000", f.now)
	f.batch(t, y, "1.000", f.now.Add(time.Hour))
	f.batch(t, z, "5.000", f.now.Add(time.Hour))
	dishes := []uuid.UUID{f.dish(t, "First", true), f.dish(t, "Second", true)}
	slices.SortFunc(dishes, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	f.recipe(t, dishes[0], 1, map[uuid.UUID]string{x: "0.150", y: "0.200"})
	f.recipe(t, dishes[1], 1, map[uuid.UUID]string{x: "0.200", z: "1.000"})
	body := orderBody(
		t,
		dto.OrderRequestItem{DishID: dishes[1].String(), Portions: 1},
		dto.OrderRequestItem{DishID: dishes[0].String(), Portions: 1},
		dto.OrderRequestItem{DishID: dishes[0].String(), Portions: 1},
	)
	response := f.createRequest(body, uuid.New())
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var result dto.OrderDetail
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID == uuid.Nil || result.Number == "" || result.Status != "completed" || result.CreatedBy.ID != f.actor ||
		result.CreatedBy.Name != "Operator" ||
		!result.CreatedAt.Equal(f.now) ||
		len(result.Items) != 2 ||
		response.Header().Get("Location") != "/api/v1/orders/"+result.ID.String() {
		t.Fatalf("order projection: %+v", result)
	}
	if result.Items[0].DishID != dishes[0] || result.Items[0].Portions != 2 || result.Items[1].DishID != dishes[1] ||
		len(result.Items[0].Allocations) != 2 ||
		len(result.Items[1].Allocations) != 3 {
		t.Fatalf("merged items/all ingredients: %+v", result.Items)
	}
	shared := []dto.OrderAllocation{}
	for _, item := range result.Items {
		var snapshot string
		var portions int
		if err := f.pool.QueryRow(f.ctx, `SELECT dish_name_snapshot,portions FROM order_items WHERE id=$1 AND recipe_version_id=(SELECT id FROM recipe_versions WHERE dish_id=$2 AND version=$3)`, item.ID, item.DishID, *item.RecipeVersion).
			Scan(&snapshot, &portions); err != nil {
			t.Fatal(err)
		}
		if snapshot != item.DishName || portions != item.Portions {
			t.Fatal("item snapshot not saved")
		}
		for _, a := range item.Allocations {
			var matches bool
			if err := f.pool.QueryRow(f.ctx, `SELECT batch_id=$2 AND order_item_id=$3 AND type='order_consumption'
				AND delta_quantity=-$4::numeric AND remaining_quantity_after=$5::numeric
				AND product_name_snapshot=$6 AND unit_snapshot::text=$7 AND actor_id=$8 AND occurred_at=$9
				FROM stock_movements WHERE id=$1`, a.MovementID, a.BatchID, item.ID, a.Quantity, a.BatchRemainingQuantityAfter, a.ProductName, a.Unit, f.actor, f.now).Scan(&matches); err != nil {
				t.Fatal(err)
			}
			if !matches || a.SupplyID != f.supply || a.SupplyNumber != "S09" || a.LineNumber < 1 {
				t.Fatalf("allocation differs from journal: %+v", a)
			}
			if a.ProductID == x {
				shared = append(shared, a)
			}
		}
	}
	if len(shared) != 3 || shared[0].Quantity != "0.300" || shared[0].BatchRemainingQuantityAfter != "0.100" ||
		shared[1].BatchID != shared[0].BatchID ||
		shared[1].BatchRemainingQuantityAfter != "0.000" ||
		shared[2].Quantity != "0.100" ||
		shared[2].BatchRemainingQuantityAfter != "0.300" {
		t.Fatalf("shared FEFO balance: %+v", shared)
	}
	for table, want := range map[string]int{"orders": 1, "order_items": 2, "stock_movements": 5, "audit_log": 1, "outbox_events": 1, "idempotency_keys": 1, "order_attempts": 0} {
		if got := f.count(t, table); got != want {
			t.Fatalf("%s: %d want %d", table, got, want)
		}
	}
	var audit model.OrderAuditSnapshot
	var auditData []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT after_data FROM audit_log WHERE entity_type='order' AND entity_id=$1 AND actor_id=$2`, result.ID, f.actor).
		Scan(&auditData); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(auditData, &audit); err != nil {
		t.Fatal(err)
	}
	if len(audit.Items) != 2 || audit.Number == 0 {
		t.Fatalf("audit: %s", auditData)
	}
	var payload []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT payload FROM outbox_events WHERE event_type='inventory.changed' AND cursor IS NULL AND published_at IS NULL`).
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event model.OutboxPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if len(event.ProductIDs) != 3 || len(event.BatchIDs) != 4 || len(event.DishIDs) != 2 || len(event.OrderIDs) != 1 ||
		event.OrderIDs[0] != result.ID ||
		!slices.Contains(event.Topics, model.TopicOrders) {
		t.Fatalf("outbox: %s", payload)
	}
}
