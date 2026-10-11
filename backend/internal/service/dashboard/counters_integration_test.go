package dashboard

import (
	"testing"
	"time"
)

func TestWarehouseCountersIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	stocked := f.product(t, "Stocked")
	expired := f.product(t, "Expired only")
	empty := f.product(t, "Empty inactive")
	f.exec(t, `UPDATE products SET is_active=false WHERE id=$1`, empty)
	// Counters cover the whole warehouse, including batches outside the preview.
	for range 30 {
		f.batch(t, stocked, "1.000", f.now.Add(time.Hour))
	}
	f.batch(t, stocked, "1.000", f.now.Add(24*time.Hour))
	f.batch(t, stocked, "1.000", f.now.Add(24*time.Hour+time.Microsecond))
	f.batch(t, stocked, "0.000", f.now.Add(time.Hour))
	f.batch(t, expired, "3.000", f.now)
	f.batch(t, expired, "1.000", f.now.Add(-time.Microsecond))
	f.batch(t, expired, "0.000", f.now.Add(-time.Hour))
	f.dish(t, "Available", true, testIngredient{stocked, "32.000"})
	f.dish(t, "Short", true, testIngredient{stocked, "32.001"})
	f.dish(t, "Expired ingredient", true, testIngredient{expired, "0.001"})
	f.dish(t, "No recipe", true)
	f.dish(t, "Inactive dish", false, testIngredient{empty, "1.000"})
	latest := f.dish(t, "Latest recipe", true, testIngredient{empty, "1.000"})
	f.recipe(t, latest, 2, testIngredient{stocked, "1.000"})
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		result := f.get(t, role)
		if result.ExpiringBatchesCount != 31 || result.ExpiredBatchesCount != 2 ||
			result.StopListDishesCount != 3 || result.OutOfStockProductsCount != 2 {
			t.Fatalf("%s: warehouse counters: %+v", role, result)
		}
	}
}

func TestEmptyWarehouseAndThresholdIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	result := f.get(t, "manager")
	if result.ExpiringBatchesCount != 0 || result.ExpiredBatchesCount != 0 ||
		result.StopListDishesCount != 0 || result.OutOfStockProductsCount != 0 {
		t.Fatalf("empty warehouse: %+v", result)
	}
	product := f.product(t, "Product")
	f.batch(t, product, "1.000", f.now.Add(2*time.Hour))
	f.batch(t, product, "1.000", f.now.Add(2*time.Hour+time.Microsecond))
	f.exec(t, `INSERT INTO restaurant_settings(id,restaurant_name,expiring_threshold_hours) VALUES(1,'Test Bistro',2)`)
	if result := f.get(t, "manager"); result.ExpiringBatchesCount != 1 {
		t.Fatalf("configured inclusive threshold: %+v", result)
	}
}
