package forecast

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	writeoffrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"go.uber.org/zap"
)

func TestForecastRealOrdersReplayRefusalCheckAndWriteOff(t *testing.T) {
	f := newFixture(t)
	pcs, batch := uuid.New(), uuid.New()
	f.exec(
		t,
		`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Pieces','pcs','pcs')`,
		pcs,
		f.category,
	)
	f.exec(
		t,
		`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,10,10,'explicit',$4)`,
		batch,
		pcs,
		f.supply,
		f.now.Add(1000*time.Hour),
	)
	f.exec(
		t,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,1)`,
		f.recipe,
		pcs,
	)
	base := repository.NewRepository(f.pool, zap.NewNop())
	order := orderservice.NewCreateService(orderrepo.NewRepository(), base, idempotency.NewService(base))
	check := orderservice.NewService(menurepo.NewRepository(), base)
	version := 1
	basket := dto.OrderRequest{
		Items: []dto.OrderRequestItem{{DishID: f.dish.String(), RecipeVersion: &version, Portions: 7}},
	}
	body, err := json.Marshal(basket)
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	created, err := order.CreateIdempotent(f.actor(), f.user, key, body)
	if err != nil || created.Status != 201 {
		t.Fatal(created, err)
	}

	if err := f.pool.QueryRow(f.ctx, `SELECT MAX(occurred_at) FROM stock_movements WHERE type='order_consumption'`).
		Scan(&f.now); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Microsecond)
	initial := f.forecast(t)
	if initial.Confidence != "low" || initial.Cause != "estimated_demand" || initial.EstimatedStopAt == nil ||
		!initial.EstimatedStopAt.Equal(f.now.Add(48*time.Hour+1)) {
		t.Fatal(initial)
	}
	state := f.state(t)
	for repeat := 0; repeat < 2; repeat++ {
		replay, err := order.CreateIdempotent(f.actor(), f.user, key, body)
		if err != nil || !bytes.Equal(created.Body, replay.Body) || replay.Status != 201 {
			t.Fatal(replay, err)
		}
		f.forecast(t)
	}
	if f.state(t) != state {
		t.Fatal("replay or forecast changed facts")
	}
	basket.Items[0].Portions = 8
	for repeat := 0; repeat < 2; repeat++ {
		if r, err := check.Check(f.actor(), basket); err != nil || r.CanCreate {
			t.Fatal(r, err)
		}
	}
	if f.state(t) != state {
		t.Fatal("check changed facts")
	}
	body, err = json.Marshal(basket)
	if err != nil {
		t.Fatal(err)
	}
	refused, err := order.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
	if err != nil || refused.Status != 409 || !bytes.Contains(refused.Body, []byte("INSUFFICIENT_STOCK")) {
		t.Fatal(refused, err)
	}
	if got := f.forecast(t); got.EstimatedStopAt == nil || !got.EstimatedStopAt.Equal(*initial.EstimatedStopAt) {
		t.Fatal("refusal counted as consumption", got)
	}
	writeoff := writeoffservice.NewService(writeoffrepo.NewRepository(), base, idempotency.NewService(base))
	offBody := []byte(`{"batch_id":"` + f.batch.String() + `","quantity":"10","reason":"Damage"}`)
	off, err := writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), offBody)
	if err != nil || off.Status != 201 {
		t.Fatal(off, err)
	}
	state = f.state(t)
	got := f.forecast(t)
	if got.Cause != "estimated_demand" || *got.LimitingProductID != f.product ||
		!got.EstimatedStopAt.Equal(f.now.Add(24*time.Hour+1)) {
		t.Fatal("stock recalculation or writeoff counted as history", got)
	}
	if f.state(t) != state {
		t.Fatal("forecast wrote after real mutations")
	}
}
