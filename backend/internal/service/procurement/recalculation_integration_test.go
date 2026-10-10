package procurement

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	procurementrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/procurement"
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func (f *procurementFixture) receipt(t *testing.T, product uuid.UUID, amount string) {
	t.Helper()
	expires := f.now.Add(time.Hour).Format(time.RFC3339Nano)
	body, err := json.Marshal(dto.CreateSupplyRequest{Number: uuid.NewString(), ReceivedAt: f.now.Format(time.RFC3339Nano),
		Items: []dto.CreateSupplyItem{{ProductID: product.String(), Quantity: amount, ExpiresAt: &expires}}})
	if err != nil {
		t.Fatal(err)
	}
	base := repository.NewRepository(f.pool, zap.NewNop())
	s := supplyservice.NewService(supplyrepository.NewRepository(), base, idempotency.NewService(base))
	result, err := s.Receive(f.ctx, f.actor, uuid.New(), body)
	if err != nil || result.Status != http.StatusCreated {
		t.Fatalf("receipt: %+v %v", result, err)
	}
}

func TestProcurementIntegrationReceiptAndOrderRecalculate(t *testing.T) {
	f := newProcurementFixture(t)
	target := "3.000"
	product := f.product(t, "Ingredient", &target)
	f.batch(t, product, "10.000", f.now)
	dish := f.dish(t, "Dish", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	check := func(wantAvailable, wantPurchase string, wantAffected int) {
		t.Helper()
		result := f.list(t, "")
		if wantPurchase == "" {
			if result.Total != 0 || len(result.Items) != 0 {
				t.Fatalf("at target still recommended: %+v", result)
			}
			return
		}
		if result.Total != 1 || len(result.Items) != 1 {
			t.Fatalf("recommendations: %+v", result)
		}
		item := result.Items[0]
		if item.AvailableQuantity != wantAvailable || item.RecommendedQuantity != wantPurchase || item.ExpiredQuantity != "10.000" || len(item.AffectedDishes) != wantAffected {
			t.Fatalf("recalculation: %+v", item)
		}
	}
	check("0.000", "3.000", 1)
	f.receipt(t, product, "2.000")
	check("2.000", "1.000", 0)
	f.receipt(t, product, "1.000")
	check("3.000", "", 0)
	base := repository.NewRepository(f.pool, zap.NewNop())
	orders := orderservice.NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	key := uuid.New()
	body, err := json.Marshal(dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	order, err := orders.CreateIdempotent(f.ctx, f.actor, key, body)
	if err != nil || order.Status != http.StatusCreated {
		t.Fatalf("order: %+v %v", order, err)
	}
	check("0.000", "3.000", 1)
	if replay, err := orders.CreateIdempotent(f.ctx, f.actor, key, body); err != nil || string(replay.Body) != string(order.Body) {
		t.Fatalf("order replay: %+v %v", replay, err)
	}
	check("0.000", "3.000", 1)
}

type stockHook struct {
	Repository
	afterLoad func()
}

func (h stockHook) Load(ctx context.Context, tx repository.DBTX, filter model.ProcurementFilter, now time.Time) ([]model.ProcurementStock, error) {
	items, err := h.Repository.Load(ctx, tx, filter, now)
	if err == nil {
		h.afterLoad()
	}
	return items, err
}

func TestProcurementIntegrationStockAndDishesUseOneSnapshot(t *testing.T) {
	f := newProcurementFixture(t)
	target := "5.000"
	product := f.product(t, "Ingredient", &target)
	dish := f.dish(t, "Dish", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	clockCalls, writes := 0, 0
	f.service.now = func() time.Time { clockCalls++; return f.now.Add(time.Duration(clockCalls-1) * time.Hour) }
	f.service.repo = stockHook{Repository: procurementrepository.NewRepository(), afterLoad: func() {
		writes++
		f.receipt(t, product, "2.000")
		f.recipe(t, dish, 2, map[uuid.UUID]string{product: "2.000"})
	}}
	before := f.list(t, "")
	if before.Total != 1 || before.Items[0].AvailableQuantity != "0.000" || before.Items[0].RecommendedQuantity != "5.000" || len(before.Items[0].AffectedDishes) != 1 || clockCalls != 1 || writes != 1 {
		t.Fatalf("mixed snapshot: %+v clock %d writes %d", before, clockCalls, writes)
	}
	f.service.repo = procurementrepository.NewRepository()
	f.service.now = func() time.Time { return f.now }
	after := f.list(t, "")
	if after.Total != 1 || after.Items[0].AvailableQuantity != "2.000" || after.Items[0].RecommendedQuantity != "3.000" || len(after.Items[0].AffectedDishes) != 0 {
		t.Fatalf("new snapshot did not see committed receipt/recipe: %+v", after)
	}
}
