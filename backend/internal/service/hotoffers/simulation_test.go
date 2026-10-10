package hotoffers

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func testDish(id, product uuid.UUID, required, available string) model.Dish {
	version := 1
	return model.Dish{
		ID: id, Name: "Блюдо", IsActive: true, RecipeVersion: &version,
		Recipe: []model.RecipeIngredient{{
			ProductID: product, ProductName: "Моцарелла", Unit: "g",
			QuantityPerPortion: required, AvailableQuantity: available, ExpiredQuantity: "0.000",
		}},
	}
}

func TestSimulationUsesOnlyConsumedExpiringBatches(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	product := uuid.New()
	batch := func(id, amount string, expiry, receipt time.Time) model.OrderBatch {
		return model.OrderBatch{
			ID:                uuid.MustParse(id),
			ProductID:         product,
			RemainingQuantity: amount,
			ExpiresAt:         expiry,
			ReceivedAt:        receipt,
		}
	}
	batches := []model.OrderBatch{
		batch("00000000-0000-0000-0000-000000000006", "10.000", now.Add(25*time.Hour), now),
		batch("00000000-0000-0000-0000-000000000005", "8.000", now.Add(2*time.Hour), now),
		batch("00000000-0000-0000-0000-000000000003", "0.300", now.Add(time.Hour), now.Add(-time.Hour)),
		batch("00000000-0000-0000-0000-000000000002", "0.200", now.Add(time.Hour), now.Add(-time.Hour)),
		batch("00000000-0000-0000-0000-000000000004", "0.100", now.Add(time.Hour), now.Add(-2*time.Hour)),
		batch("00000000-0000-0000-0000-000000000001", "50.000", now, now),
	}
	before := append([]model.OrderBatch{}, batches...)
	dishes := []model.Dish{
		testDish(uuid.New(), product, "0.500", "18.600"),
		testDish(uuid.New(), product, "0.500", "18.600"),
	}
	offers, err := simulate(dishes, batches, now, 24)
	if err != nil || len(offers) != 2 {
		t.Fatalf("offers: %+v, %v", offers, err)
	}
	for _, offer := range offers {
		if offer.availablePortions != "37" || len(offer.problems) != 3 {
			t.Fatalf("one-portion simulation: %+v", offer)
		}
		for i, want := range []struct {
			index int
			used  string
		}{{4, "0.100"}, {3, "0.200"}, {2, "0.200"}} {
			got := offer.problems[i].allocation
			if got.Batch.ID != batches[want.index].ID || got.Quantity != want.used {
				t.Fatalf("FEFO allocation %d: %+v", i, got)
			}
		}
	}
	if !reflect.DeepEqual(before, batches) {
		t.Fatal("simulation mutated stock")
	}
}

func TestSimulationEligibilityAndThreshold(t *testing.T) {
	now := time.Now().UTC()
	product := uuid.New()
	for _, tc := range []struct {
		name             string
		delta            time.Duration
		amount, required string
		active           bool
		want             int
	}{
		{"inclusive threshold", 24 * time.Hour, "1.000", "1.000", true, 1},
		{"later", 24*time.Hour + time.Nanosecond, "1.000", "1.000", true, 0},
		{"expired", 0, "0.000", "1.000", true, 0},
		{"depleted", time.Hour, "0.000", "1.000", true, 0},
		{"shortage", time.Hour, "0.999", "1.000", true, 0},
		{"inactive", time.Hour, "1.000", "1.000", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dish := testDish(uuid.New(), product, tc.required, tc.amount)
			dish.IsActive = tc.active
			batches := []model.OrderBatch{
				{ID: uuid.New(), ProductID: product, RemainingQuantity: tc.amount, ExpiresAt: now.Add(tc.delta)},
			}
			got, err := simulate([]model.Dish{dish}, batches, now, 24)
			if err != nil || len(got) != tc.want {
				t.Fatalf("eligibility: %+v, %v", got, err)
			}
		})
	}
	for _, hours := range []int{0, 721} {
		if _, err := simulate(nil, nil, now, hours); err == nil {
			t.Fatal("accepted invalid threshold")
		}
	}
	missing := testDish(uuid.New(), product, "1", "1")
	missing.Recipe = nil
	if got, err := simulate([]model.Dish{missing}, nil, now, 24); err != nil || len(got) != 0 {
		t.Fatalf("missing recipe: %+v, %v", got, err)
	}
	broken := testDish(uuid.New(), product, "1", "broken")
	if _, err := simulate([]model.Dish{broken}, nil, now, 24); err == nil {
		t.Fatal("accepted broken recipe stock")
	}
	broken = testDish(uuid.New(), product, "1", "1")
	if _, err := simulate([]model.Dish{broken}, nil, now, 24); err == nil {
		t.Fatal("accepted inconsistent FEFO stock")
	}
}
