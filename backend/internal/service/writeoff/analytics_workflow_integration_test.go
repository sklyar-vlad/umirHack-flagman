package writeoff_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	"go.uber.org/zap"
)

type analyticsWorkflowState struct {
	Grams, Pieces                                    string
	Movements, Attempts, Orders, Audit, Outbox, Keys int
}

func (f *fixture) analyticsState(t *testing.T, pcsBatch uuid.UUID) analyticsWorkflowState {
	t.Helper()
	var s analyticsWorkflowState
	err := f.pool.QueryRow(f.ctx, `SELECT (SELECT remaining_quantity::text FROM batches WHERE id=$1),(SELECT remaining_quantity::text FROM batches WHERE id=$2),(SELECT count(*) FROM stock_movements),(SELECT count(*) FROM order_attempts),(SELECT count(*) FROM orders),(SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_keys)`, f.batch, pcsBatch).
		Scan(&s.Grams, &s.Pieces, &s.Movements, &s.Attempts, &s.Orders, &s.Audit, &s.Outbox, &s.Keys)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAnalyticsRealOrderRefusalReplayCheckAndBothWriteOffs(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `UPDATE products SET name='Мука',unit='g',display_unit='g' WHERE id=$1`, f.product)
	f.exec(t, `UPDATE stock_movements SET product_name_snapshot='Мука',unit_snapshot='g'`)
	egg, eggBatch := uuid.New(), uuid.New()
	f.exec(
		t,
		`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Яйцо','pcs','pcs')`,
		egg,
		f.category,
	)
	f.exec(
		t,
		`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,8,8,'explicit',clock_timestamp()+interval '1 day')`,
		eggBatch,
		egg,
		f.supply,
	)
	f.exec(
		t,
		`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at) VALUES($1,'receipt',8,8,'Яйцо','pcs',$2,clock_timestamp()-interval '1 hour')`,
		eggBatch,
		f.user,
	)
	body := f.orderBody(t, "2.001")
	var basket dto.OrderRequest
	if err := json.Unmarshal(body, &basket); err != nil {
		t.Fatal(err)
	}
	f.exec(
		t,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) SELECT id,$2,2 FROM recipe_versions WHERE dish_id=$1`,
		basket.Items[0].DishID,
		egg,
	)
	base := repository.NewRepository(f.pool, zap.NewNop())
	order := orderservice.NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	check := orderservice.NewService(menurepository.NewRepository(), base)
	created, err := order.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
	if err != nil || created.Status != 201 {
		t.Fatal(created, err)
	}
	state := f.analyticsState(t, eggBatch)
	if state.Grams != "7.999" || state.Pieces != "6.000" || state.Orders != 1 {
		t.Fatal(state)
	}
	if items, n := f.losses(t, model.AnalyticsFilter{}); n != 0 || len(items) != 0 {
		t.Fatal("order consumption counted as loss", items, n)
	}
	if items, n := f.shortages(t, model.AnalyticsFilter{}); n != 0 || len(items) != 0 {
		t.Fatal("successful demand counted", items, n)
	}
	basket.Items[0].Portions = 5
	refusalBody, err := json.Marshal(basket)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		checked, err := check.Check(f.actor(), basket)
		if err != nil || checked.CanCreate || len(checked.Shortages) != 2 {
			t.Fatal(checked, err)
		}
	}
	if got := f.analyticsState(t, eggBatch); got != state {
		t.Fatal("checks wrote facts", got, state)
	}
	key := uuid.New()
	refused, err := order.CreateIdempotent(f.actor(), f.user, key, refusalBody)
	if err != nil || refused.Status != 409 || !bytes.Contains(refused.Body, []byte("INSUFFICIENT_STOCK")) {
		t.Fatal(refused, err)
	}
	assertShortages := func(count int64, grams, pieces string) {
		t.Helper()
		items, n := f.shortages(t, model.AnalyticsFilter{})
		if n != 2 || len(items) != 2 {
			t.Fatal(items, n)
		}
		for _, item := range items {
			want, unit, name := grams, "g", "Мука"
			if item.ProductID == egg {
				want, unit, name = pieces, "pcs", "Яйцо"
			} else if item.ProductID != f.product {
				t.Fatal(item)
			}
			if item.TotalMissingQuantity != want || item.Unit != unit || item.ProductName != name ||
				item.RejectedAttemptsCount != count {
				t.Fatal(item)
			}
		}
	}
	assertShortages(1, "2.006", "4.000")
	afterRefusal := f.analyticsState(t, eggBatch)
	for i := 0; i < 3; i++ {
		replay, err := order.CreateIdempotent(f.actor(), f.user, key, refusalBody)
		if err != nil || replay.Status != refused.Status || !bytes.Equal(replay.Body, refused.Body) {
			t.Fatal("replay", replay, err)
		}
		if _, err := check.Check(f.actor(), basket); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.analyticsState(t, eggBatch); got != afterRefusal || got.Attempts != 1 {
		t.Fatal("replay or check inflation", got, afterRefusal)
	}
	assertShortages(1, "2.006", "4.000")
	// A different key is another real rejected request, even with the same body.
	again, err := order.CreateIdempotent(f.actor(), f.user, uuid.New(), refusalBody)
	if err != nil || again.Status != 409 {
		t.Fatal(again, err)
	}
	assertShortages(2, "4.012", "8.000")
	// Valid gram write-off and replay, followed by an expired pcs manual write-off.
	offKey := uuid.New()
	off := f.create(t, "1.001", offKey)
	replay := f.create(t, "1.001", offKey)
	if off.Status != 201 || !bytes.Equal(off.Body, replay.Body) {
		t.Fatal("manual replay", off, replay)
	}
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, eggBatch)
	pcsOffBody := []byte(`{"batch_id":"` + eggBatch.String() + `","quantity":"1","reason":"Повреждение"}`)
	pcsOff, err := f.writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), pcsOffBody)
	if err != nil || pcsOff.Status != 201 {
		t.Fatal(pcsOff, err)
	}
	losses, n := f.losses(t, model.AnalyticsFilter{})
	if n != 2 {
		t.Fatal(losses, n)
	}
	for _, item := range losses {
		want := "1.001"
		if item.ProductID == egg {
			want = "1.000"
		}
		if item.ExpiredWrittenOffQuantity != "0.000" || item.ManualWrittenOffQuantity != want ||
			item.TotalWrittenOffQuantity != want ||
			item.MovementsCount != 1 {
			t.Fatal(item)
		}
	}
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	f.enable(t, true)
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	expired := f.analyticsState(t, eggBatch)
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.analyticsState(t, eggBatch); got != expired || got.Grams != "0.000" || got.Pieces != "0.000" {
		t.Fatal("repeat expiry", got, expired)
	}
	// Catalog edits cannot alter report facts, and report reads cannot write facts.
	f.exec(
		t,
		`UPDATE products SET name='Changed '||id::text,is_active=false WHERE id=ANY($1::uuid[])`,
		[]uuid.UUID{f.product, egg},
	)
	for i := 0; i < 3; i++ {
		losses, n = f.losses(t, model.AnalyticsFilter{})
		if n != 2 || len(losses) != 2 {
			t.Fatal(losses, n)
		}
		for _, item := range losses {
			manual, total, auto, unit, name := "1.001", "7.999", "6.998", "g", "Мука"
			if item.ProductID == egg {
				manual, total, auto, unit, name = "1.000", "6.000", "5.000", "pcs", "Яйцо"
			}
			if item.ManualWrittenOffQuantity != manual || item.ExpiredWrittenOffQuantity != auto ||
				item.TotalWrittenOffQuantity != total ||
				item.Unit != unit ||
				item.ProductName != name ||
				item.MovementsCount != 2 {
				t.Fatal(item)
			}
		}
		assertShortages(2, "4.012", "8.000")
	}
	if got := f.analyticsState(
		t,
		eggBatch,
	); got != expired || got.Attempts != 2 || got.Orders != 1 ||
		got.Movements != 8 {
		t.Fatal("analytics wrote or inflated facts", got, expired)
	}
}
