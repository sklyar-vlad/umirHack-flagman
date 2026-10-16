package dashboard

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAttentionPreviewsAndRoleBlocksIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	f.exec(t, `INSERT INTO restaurant_settings(id,restaurant_name,timezone,expiring_threshold_hours)
		VALUES(1,'Бистро №12','Europe/Moscow',4)`)
	expiredProduct := f.product(t, "Expired")
	expiredBatch := f.batch(t, expiredProduct, "2.000", f.now.Add(-time.Microsecond))
	problemProduct := f.product(t, "Attention")
	ids := make([]uuid.UUID, 0, 25)
	for range 25 {
		ids = append(ids, f.batch(t, problemProduct, "1.000", f.now.Add(30*time.Minute)))
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	f.batch(t, problemProduct, "0.000", f.now)
	f.batch(t, problemProduct, "1.000", f.now.Add(4*time.Hour+time.Microsecond))
	var dishIDs []uuid.UUID
	var procurementIDs []uuid.UUID
	for i := 1; i <= 4; i++ {
		p := f.product(t, fmt.Sprintf("Offer %d", i))
		f.batch(t, p, "10.000", f.now.Add(time.Hour))
		dishIDs = append(
			dishIDs,
			f.dish(t, fmt.Sprintf("Dish %d", i), true, testIngredient{p, fmt.Sprintf("%d.000", i)}),
		)
		purchase := f.product(t, fmt.Sprintf("Purchase %d", i))
		f.exec(t, `UPDATE products SET target_stock='20.000' WHERE id=$1`, purchase)
		procurementIDs = append(procurementIDs, purchase)
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		result := f.response(t)
		if result.ExpiringBatchesCount != 29 || result.ExpiredBatchesCount != 1 || result.AttentionTotal != 30 ||
			len(result.AttentionBatches) != 20 || result.OutOfStockProductsCount != 5 {
			t.Fatalf("%s: totals or limits: %+v", role, result)
		}
		if result.Settings.RestaurantName != "Бистро №12" || result.Settings.Timezone != "Europe/Moscow" ||
			result.Settings.ExpiringThresholdHours != 4 {
			t.Fatalf("shell settings: %+v", result.Settings)
		}
		first := result.AttentionBatches[0]
		if first.BatchID != expiredBatch || first.Status != "expired" || first.RemainingSeconds != -1 ||
			first.ProductID != expiredProduct || first.ProductName != "Expired" || first.Unit != "g" ||
			first.SupplyID != f.supply || first.SupplyNumber != "S14-1" || first.LineNumber != 1 ||
			first.RemainingQuantity != "2.000" || first.ExpiresAt != f.now.Add(-time.Microsecond).Format(time.RFC3339Nano) {
			t.Fatalf("attention batch facts: %+v", first)
		}
		for i, b := range result.AttentionBatches[1:] {
			if b.BatchID != ids[i] || b.Status != "expiring" || b.RemainingSeconds != 1800 {
				t.Fatalf("stable attention priority %d: %+v", i, b)
			}
		}
		if role == "storekeeper" {
			if result.HotOffers == nil || len(result.HotOffers) != 0 {
				t.Fatal("storekeeper offers must be []")
			}
		} else {
			if len(result.HotOffers) != 3 {
				t.Fatalf("offers limit: %+v", result.HotOffers)
			}
			for i, offer := range result.HotOffers {
				if offer.Dish.ID != dishIDs[3-i] || offer.Rank != int64(i+1) ||
					offer.PriorityScore != []string{"30.0000", "22.5000", "15.0000"}[i] || offer.PriorityLevel != "high" {
					t.Fatalf("shared offer priority: %+v", offer)
				}
			}
		}
		if role == "order_operator" {
			if result.ProcurementRecommendations == nil || len(result.ProcurementRecommendations) != 0 {
				t.Fatal("operator procurement must be []")
			}
		} else {
			if len(result.ProcurementRecommendations) != 3 {
				t.Fatal("procurement limit")
			}
			for i, item := range result.ProcurementRecommendations {
				if item.ProductID != procurementIDs[i] || item.RecommendedQuantity != "20.000" ||
					item.AffectedDishes == nil {
					t.Fatalf("shared procurement priority: %+v", item)
				}
			}
		}
	}
}

func TestDashboardSessionAndValidationIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	if response := f.request("?page=bad", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("session must precede parsing: %d", response.Code)
	}
	for _, query := range []string{"?page=1", "?search=%zz", "?unknown="} {
		if response := f.request(query, true); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("query %q: %d %s", query, response.Code, response.Body.String())
		}
	}
	f.exec(t, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE user_id=$1`, f.actor)
	if response := f.request("", true); response.Code != http.StatusUnauthorized {
		t.Fatal("revoked session allowed")
	}
}
