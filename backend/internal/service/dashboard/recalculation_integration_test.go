package dashboard

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
	dashboardrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func (f *dashboardFixture) receipt(t *testing.T, product uuid.UUID, amount string, expires time.Time) {
	t.Helper()
	expiry := expires.Format(time.RFC3339Nano)
	body, err := json.Marshal(
		dto.CreateSupplyRequest{
			Number:     uuid.NewString(),
			ReceivedAt: f.now.Format(time.RFC3339Nano),
			Items:      []dto.CreateSupplyItem{{ProductID: product.String(), Quantity: amount, ExpiresAt: &expiry}},
		},
	)
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

func (f *dashboardFixture) order(t *testing.T, dish uuid.UUID, portions int) {
	t.Helper()
	base := repository.NewRepository(f.pool, zap.NewNop())
	s := orderservice.NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	body, err := json.Marshal(
		dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: portions}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.CreateIdempotent(f.ctx, f.actor, uuid.New(), body)
	if err != nil || result.Status != http.StatusCreated {
		t.Fatalf("order: %+v %v", result, err)
	}
}

func TestDashboardReceiptOrderExpiryAndRecipeRecalculationIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	p := f.product(t, "Ingredient")
	f.exec(t, `UPDATE products SET target_stock='3.000' WHERE id=$1`, p)
	expired := f.batch(t, p, "5.000", f.now)
	dish := f.dish(t, "Dish", true, testIngredient{p, "1.000"})
	check := func(expiring, expired, stop, missing int64, purchase, portions string) {
		t.Helper()
		r := f.response(t)
		if r.ExpiringBatchesCount != expiring || r.ExpiredBatchesCount != expired ||
			r.StopListDishesCount != stop || r.OutOfStockProductsCount != missing || r.AttentionTotal != expiring+expired {
			t.Fatalf("recalculated counters: %+v", r)
		}
		if purchase == "" {
			if len(r.ProcurementRecommendations) != 0 {
				t.Fatal("no purchase expected")
			}
		} else if len(r.ProcurementRecommendations) != 1 || r.ProcurementRecommendations[0].RecommendedQuantity != purchase ||
			len(r.ProcurementRecommendations[0].AffectedDishes) != int(stop) {
			t.Fatalf("recalculated procurement: %+v", r.ProcurementRecommendations)
		}
		if portions == "" {
			if len(r.HotOffers) != 0 {
				t.Fatal("no offer expected")
			}
		} else if len(r.HotOffers) != 1 || r.HotOffers[0].AvailablePortions != portions {
			t.Fatalf("recalculated offers: %+v", r.HotOffers)
		}
	}
	check(0, 1, 1, 1, "3.000", "")
	f.receipt(t, p, "2.000", f.now.Add(time.Hour))
	check(1, 1, 0, 0, "1.000", "2")
	f.receipt(t, p, "1.000", f.now.Add(72*time.Hour))
	check(1, 1, 0, 0, "", "3")
	f.order(t, dish, 2)
	check(0, 1, 0, 0, "2.000", "") // remaining stock is available but outside threshold.
	f.order(t, dish, 1)
	check(0, 1, 1, 1, "3.000", "")
	f.receipt(t, p, "1.000", f.now.Add(time.Hour))
	check(1, 1, 0, 0, "2.000", "1")
	f.recipe(t, dish, 2, testIngredient{p, "2.000"})
	check(1, 1, 1, 0, "2.000", "")
	f.now = f.now.Add(time.Hour)
	check(0, 2, 1, 1, "3.000", "")
	if r := f.get(t, "manager"); len(r.AttentionBatches) != 2 || r.AttentionBatches[0].BatchID != expired {
		t.Fatalf("expiry keeps attention facts: %+v", r.AttentionBatches)
	}
}

type stockHook struct {
	Repository
	afterLoad func()
}

func (h stockHook) Load(ctx context.Context, tx repository.DBTX, now time.Time) (model.DashboardData, error) {
	data, err := h.Repository.Load(ctx, tx, now)
	if err == nil {
		h.afterLoad()
	}
	return data, err
}

func TestDashboardOneSnapshotAndOneTimeIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	p := f.product(t, "Ingredient")
	f.exec(t, `UPDATE products SET target_stock='5.000' WHERE id=$1`, p)
	f.exec(t, `INSERT INTO restaurant_settings(id,restaurant_name) VALUES(1,'Before')`)
	batch := f.batch(t, p, "2.000", f.now.Add(time.Hour))
	dish := f.dish(t, "Dish", true, testIngredient{p, "1.000"})
	clockCalls := 0
	f.service.now = func() time.Time { clockCalls++; return f.now.Add(time.Duration(clockCalls-1) * time.Hour) }
	f.service.repo = stockHook{Repository: dashboardrepository.NewRepository(), afterLoad: func() {
		f.order(t, dish, 2)
		f.receipt(t, p, "1.000", f.now.Add(time.Hour))
		f.recipe(t, dish, 2, testIngredient{p, "2.000"})
		f.exec(t, `UPDATE restaurant_settings SET restaurant_name='After',expiring_threshold_hours=1 WHERE id=1`)
	}}
	before := f.get(t, "manager")
	if clockCalls != 1 || before.ExpiringBatchesCount != 1 || before.StopListDishesCount != 0 ||
		before.Settings.RestaurantName != "Before" || before.Settings.ExpiringThresholdHours != 24 ||
		len(before.HotOffers) != 1 || before.HotOffers[0].Dish.RecipeVersion != 1 ||
		before.HotOffers[0].AvailablePortions != "2" || before.HotOffers[0].PriorityScore != "47.9167" ||
		before.HotOffers[0].ProblemBatches[0].BatchID != batch || before.AttentionBatches[0].BatchID != batch ||
		before.AttentionBatches[0].RemainingQuantity != "2.000" ||
		before.ProcurementRecommendations[0].RecommendedQuantity != "3.000" {
		t.Fatalf("mixed snapshot/time: %+v, clock %d", before, clockCalls)
	}
	f.service.repo = dashboardrepository.NewRepository()
	f.service.now = func() time.Time { return f.now }
	after := f.get(t, "manager")
	if after.StopListDishesCount != 1 || after.Settings.RestaurantName != "After" ||
		after.Settings.ExpiringThresholdHours != 1 ||
		len(after.HotOffers) != 0 ||
		after.AttentionBatches[0].BatchID == batch ||
		after.AttentionBatches[0].RemainingQuantity != "1.000" ||
		after.ProcurementRecommendations[0].RecommendedQuantity != "4.000" ||
		len(after.ProcurementRecommendations[0].AffectedDishes) != 1 {
		t.Fatalf("next snapshot did not see committed writes: %+v", after)
	}
}
