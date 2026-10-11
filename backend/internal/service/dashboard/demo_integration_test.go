package dashboard

import (
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestDashboardDemoDataAndReadOnlyIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	for _, file := range []string{"001_catalog.sql", "002_inventory.sql"} {
		data, err := os.ReadFile("../../../database/seeds/" + file)
		if err != nil {
			t.Fatal(err)
		}
		f.exec(t, string(data))
	}
	// Seed expiry dates are relative to its own database clock.
	if err := f.pool.QueryRow(f.ctx, `SELECT received_at FROM supplies WHERE number='105'`).Scan(&f.now); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.UTC()
	state := func() string {
		t.Helper()
		var value string
		if err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
			'batches',(SELECT jsonb_agg(to_jsonb(b) ORDER BY id) FROM batches b),
			'orders',(SELECT count(*) FROM orders),
			'attempts',(SELECT count(*) FROM order_attempts),
			'movements',(SELECT count(*) FROM stock_movements),
			'audit',(SELECT count(*) FROM audit_log),
			'outbox',(SELECT count(*) FROM outbox_events))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := state()
	r := f.response(t)
	if r.ExpiringBatchesCount != 1 || r.ExpiredBatchesCount != 1 || r.StopListDishesCount != 2 ||
		r.OutOfStockProductsCount != 2 || r.AttentionTotal != 2 || len(r.AttentionBatches) != 2 ||
		len(r.HotOffers) != 1 || len(r.ProcurementRecommendations) != 3 || r.Settings.RestaurantName != "Кладовая" {
		t.Fatalf("demo dashboard: %+v", r)
	}
	offer := r.HotOffers[0]
	if offer.Dish.ID != uuid.MustParse("30000000-0000-4000-8000-000000000001") || offer.AvailablePortions != "20" ||
		offer.PriorityScore != "1.6667" || offer.PriorityLevel != "low" ||
		offer.ProblemBatches[0].UsedForOnePortion != "120.000" {
		t.Fatalf("demo offer: %+v", offer)
	}
	for i, item := range r.ProcurementRecommendations {
		if item.ProductName != []string{"Куриное филе", "Сливки", "Макароны"}[i] ||
			item.RecommendedQuantity != []string{"5000.000", "3000.000", "1000.000"}[i] {
			t.Fatalf("demo procurement order: %+v", r.ProcurementRecommendations)
		}
	}
	if after := state(); before != after {
		t.Fatal("dashboard changed stock or event/history tables")
	}
}
