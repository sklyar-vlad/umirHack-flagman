package hotoffers

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
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func (f *offersFixture) receipt(t *testing.T, product uuid.UUID, amount string, expires time.Time) {
	t.Helper()
	expiry := expires.Format(time.RFC3339Nano)
	body, err := json.Marshal(dto.CreateSupplyRequest{Number: uuid.NewString(), ReceivedAt: f.now.Format(time.RFC3339Nano), Items: []dto.CreateSupplyItem{{ProductID: product.String(), Quantity: amount, ExpiresAt: &expiry}}})
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

func TestOffersIntegrationReceiptOrderReplayAndRecipeRecalculate(t *testing.T) {
	f := newOffersFixture(t)
	p := f.product(t, "Ingredient")
	other := f.product(t, "Later ingredient")
	f.batch(t, p, "5.000", f.now) // expired stock cannot make the dish available.
	f.batch(t, other, "10.000", f.now.Add(72*time.Hour))
	dish := f.dish(t, "Dish", true, testIngredient{p, "1.000"})
	check := func(want int64, portions string) {
		t.Helper()
		result := f.list(t, "")
		if result.Total != want || int64(len(result.Items)) != want {
			t.Fatalf("recalculation: %+v", result)
		}
		if want > 0 && result.Items[0].AvailablePortions != portions {
			t.Fatalf("portions: %+v", result)
		}
	}
	check(0, "")
	f.receipt(t, p, "1.000", f.now.Add(time.Hour))
	f.receipt(t, p, "3.000", f.now.Add(72*time.Hour))
	check(1, "4")
	base := repository.NewRepository(f.pool, zap.NewNop())
	orders := orderservice.NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	key := uuid.New()
	body, err := json.Marshal(dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	order, err := orders.CreateIdempotent(f.ctx, f.actor, key, body)
	if err != nil || order.Status != http.StatusCreated {
		t.Fatalf("order: %+v %v", order, err)
	}
	check(0, "") // still available, but no expiring batch is consumed.
	if replay, err := orders.CreateIdempotent(f.ctx, f.actor, key, body); err != nil || string(replay.Body) != string(order.Body) {
		t.Fatalf("order replay: %+v %v", replay, err)
	}
	check(0, "")
	f.receipt(t, p, "1.000", f.now.Add(time.Hour))
	check(1, "4")
	f.recipe(t, dish, 2, testIngredient{other, "1.000"})
	check(0, "")
	f.recipe(t, dish, 3, testIngredient{p, "5.000"})
	check(0, "") // insufficient current stock, despite a consumed expiring ingredient.
	f.receipt(t, p, "1.000", f.now.Add(time.Hour))
	check(1, "1")
	result := f.list(t, "")
	if result.Items[0].Dish.RecipeVersion != 3 || len(result.Items[0].ProblemBatches) != 2 {
		t.Fatalf("latest recipe: %+v", result)
	}
	f.exec(t, `UPDATE dishes SET is_active=false WHERE id=$1`, dish)
	check(0, "")
}

type menuHook struct {
	MenuRepository
	afterLoad func()
}

func (m menuHook) Load(ctx context.Context, tx repository.DBTX, filter model.DishFilter, now time.Time) ([]model.Dish, error) {
	dishes, err := m.MenuRepository.Load(ctx, tx, filter, now)
	if err == nil {
		m.afterLoad()
	}
	return dishes, err
}

func TestOffersIntegrationConsistentSnapshot(t *testing.T) {
	f := newOffersFixture(t)
	p := f.product(t, "Ingredient")
	batch := f.batch(t, p, "1.000", f.now.Add(12*time.Hour))
	dish := f.dish(t, "Dish", true, testIngredient{p, "1.000"})
	changed := false
	f.service.menu = menuHook{MenuRepository: f.service.menu, afterLoad: func() {
		if changed {
			return
		}
		changed = true
		f.receipt(t, p, "1.000", f.now.Add(12*time.Hour))
		f.recipe(t, dish, 2, testIngredient{p, "2.000"})
		f.exec(t, `INSERT INTO restaurant_settings(restaurant_name,expiring_threshold_hours)VALUES('Test',48)`)
	}}
	result := f.list(t, "")
	if result.Total != 1 || result.Items[0].Dish.RecipeVersion != 1 || result.Items[0].PriorityScore != "50.0000" || result.Items[0].PriorityLevel != "medium" || result.Items[0].AvailablePortions != "1" || len(result.Items[0].ProblemBatches) != 1 || result.Items[0].ProblemBatches[0].BatchID != batch {
		t.Fatalf("mixed snapshot: %+v", result)
	}
	result = f.list(t, "")
	if result.Total != 1 || result.Items[0].Dish.RecipeVersion != 2 || result.Items[0].PriorityScore != "75.0000" || result.Items[0].PriorityLevel != "high" || result.Items[0].AvailablePortions != "1" || len(result.Items[0].ProblemBatches) != 2 {
		t.Fatalf("new snapshot: %+v", result)
	}
}
