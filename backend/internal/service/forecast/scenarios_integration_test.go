package forecast

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	forecastrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/forecast"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	"go.uber.org/zap"
)

// Snapshot all inventory and write-side facts, not just one batch balance.
func (f *fixture) state(t *testing.T) string {
	t.Helper()
	var s string
	err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'batches',(SELECT jsonb_agg(to_jsonb(b) ORDER BY id) FROM batches b),
		'movements',(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM stock_movements m),
		'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM orders o),
		'attempts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM order_attempts a),
		'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_log a),
		'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM outbox_events o),
		'keys',(SELECT jsonb_agg(to_jsonb(k) ORDER BY id) FROM idempotency_keys k),
		'recipes',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM recipe_versions r)
	)::text`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestForecastPostgreSQLScenariosPreserveInventory(t *testing.T) {
	for _, tc := range []struct {
		name              string
		qty               string
		expiry            int
		history           bool
		later             string
		at                time.Duration
		cause, confidence string
	}{
		{"already unavailable", "9", 1000, true, "", 0, "already_unavailable", "low"},
		{"depletion without expiry", "20", 1000, true, "", 10*time.Hour + 1, "estimated_demand", "low"},
		{"expiry", "100", 3, true, "", 3 * time.Hour, "expiry", "low"},
		{"sufficient later stock", "100", 3, true, "100", -1, "unknown", "low"},
		{"demand after expiry", "100", 3, true, "20", 13*time.Hour + 1, "estimated_demand", "low"},
		{"no history", "100", 1000, false, "", -1, "unknown", "insufficient_data"},
		{"no history expiry-only", "100", 3, false, "", 3 * time.Hour, "expiry", "insufficient_data"},
		{"no history later stock", "100", 3, false, "10", -1, "unknown", "insufficient_data"},
		{"expired excluded", "100", 0, false, "", 0, "already_unavailable", "insufficient_data"},
		{"horizon inclusive", "100", 72, false, "", 72 * time.Hour, "expiry", "insufficient_data"},
		{"exact demand horizon available", "82", 1000, true, "", -1, "unknown", "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.exec(t, `UPDATE batches SET remaining_quantity=$2,expires_at=$3 WHERE id=$1`, f.batch, tc.qty, f.now.Add(time.Duration(tc.expiry)*time.Hour))
			if tc.history {
				f.consumption(t, "168", "g", f.now.Add(-time.Hour))
			}
			if tc.later != "" {
				f.exec(t, `INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,$4,$4,'explicit',$5)`, uuid.New(), f.product, f.supply, tc.later, f.now.Add(1000*time.Hour))
			}
			before := f.state(t)
			for repeat := 0; repeat < 2; repeat++ {
				i := f.forecast(t)
				if i.Cause != tc.cause || i.Confidence != tc.confidence || tc.at < 0 && i.EstimatedStopAt != nil || tc.at >= 0 && (i.EstimatedStopAt == nil || !i.EstimatedStopAt.Equal(f.now.Add(tc.at))) {
					t.Fatal(i, "want", tc)
				}
				if i.EstimatedStopAt != nil && (i.LimitingProductID == nil || *i.LimitingProductID != f.product) {
					t.Fatal(i)
				}
			}
			if after := f.state(t); after != before {
				t.Fatal("forecast changed stored facts")
			}
		})
	}
}

type interleavingRepository struct {
	*forecastrepo.Repository
	mutate func()
	t      *testing.T
}

func (r *interleavingRepository) Batches(ctx context.Context, q repository.DBTX, products []uuid.UUID, now time.Time) ([]model.OrderBatch, error) {
	var iso, mode string
	if err := q.QueryRow(ctx, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only')`).Scan(&iso, &mode); err != nil {
		r.t.Fatal(err)
	}
	if iso != "repeatable read" || mode != "on" {
		r.t.Fatal(iso, mode)
	}
	r.mutate()
	return r.Repository.Batches(ctx, q, products, now)
}
func TestForecastRecipesStockAndHistoryShareReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	f.consumption(t, "168", "g", f.now.Add(-time.Hour))
	f.exec(t, `UPDATE batches SET remaining_quantity=20 WHERE id=$1`, f.batch)
	before, err := f.service().List(f.actor(), filter())
	if err != nil {
		t.Fatal(err)
	}
	r := &interleavingRepository{Repository: forecastrepo.NewRepository(), t: t, mutate: func() {
		f.exec(t, `UPDATE batches SET remaining_quantity=1,expires_at=$2 WHERE id=$1`, f.batch, f.now.Add(time.Hour))
		f.consumption(t, "336", "g", f.now.Add(-time.Hour))
		rv := uuid.New()
		f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,2,$3)`, rv, f.dish, f.user)
		f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,2)`, rv, f.product)
		f.exec(t, `UPDATE dishes SET name='New name' WHERE id=$1`, f.dish)
	}}
	s := NewService(r, menurepo.NewRepository(), repository.NewRepository(f.pool, zap.NewNop()))
	s.now = func() time.Time { return f.now }
	got, err := s.List(f.actor(), filter())
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatal("mixed snapshot", got, before, err)
	}
	next, err := f.service().List(f.actor(), filter())
	if err != nil || next.Items[0].DishName != "New name" || next.Items[0].Cause != "already_unavailable" {
		t.Fatal(next, err)
	}
}

func TestForecastLimitingProductTieAndPartialHistory(t *testing.T) {
	f := newFixture(t)
	f.consumption(t, "168", "g", f.now.Add(-time.Hour))
	f.exec(t, `UPDATE batches SET remaining_quantity=20 WHERE id=$1`, f.batch)
	other := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	f.exec(t, `INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Other ingredient','pcs','pcs')`, other, f.category)
	f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,1)`, f.recipe, other)
	f.exec(t, `INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,2,2,'explicit',$4)`, uuid.New(), other, f.supply, f.now.Add(1000*time.Hour))
	i := f.forecast(t)
	if i.Confidence != "insufficient_data" || i.Cause != "estimated_demand" || *i.LimitingProductID != f.product {
		t.Fatal(i)
	}
	// Equal expiry stops are stable regardless of recipe/product name order.
	f.exec(t, `UPDATE batches SET expires_at=$1`, f.now.Add(time.Hour))
	i = f.forecast(t)
	if i.Cause != "expiry" || *i.LimitingProductID != other {
		t.Fatal(i)
	}
	// One snapshot timestamp across a non-UTC input clock.
	s := f.service()
	s.now = func() time.Time { return f.now.In(time.FixedZone("MSK", 3*3600)) }
	got, err := s.List(f.actor(), filter())
	if err != nil || got.Items[0].EstimatedStopAt.Location() != time.UTC {
		t.Fatal(got, err)
	}
}
