package forecast

import (
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"math/big"
	"reflect"
	"testing"
	"time"
)

func TestIngredientFEFOEventSimulation(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	batch := func(q string, hours int) model.OrderBatch {
		return model.OrderBatch{ID: uuid.New(), ProductID: uuid.New(), RemainingQuantity: q, ExpiresAt: now.Add(time.Duration(hours) * time.Hour), ReceivedAt: now.Add(-time.Hour)}
	}
	rate := new(big.Rat).SetFrac(big.NewInt(1000), big.NewInt(int64(time.Hour))) // 1 base unit / hour
	for _, tc := range []struct {
		name    string
		batches []model.OrderBatch
		need    int64
		rate    *big.Rat
		horizon time.Duration
		at      time.Duration
		cause   string
	}{
		{"already", []model.OrderBatch{batch("9", 100)}, 10000, rate, 72 * time.Hour, 0, "already_unavailable"},
		{"demand", []model.OrderBatch{batch("20", 100)}, 10000, rate, 72 * time.Hour, 10*time.Hour + 1, "estimated_demand"},
		{"expiry", []model.OrderBatch{batch("100", 3)}, 10000, rate, 72 * time.Hour, 3 * time.Hour, "expiry"},
		{"later sufficient", []model.OrderBatch{batch("100", 3), batch("100", 100)}, 10000, rate, 72 * time.Hour, -1, "unknown"},
		{"depletion to later", []model.OrderBatch{batch("2", 3), batch("20", 100)}, 10000, rate, 72 * time.Hour, 12*time.Hour + 1, "estimated_demand"},
		{"expiry then demand", []model.OrderBatch{batch("100", 3), batch("20", 100)}, 10000, rate, 72 * time.Hour, 13*time.Hour + 1, "estimated_demand"},
		{"no history expiry", []model.OrderBatch{batch("20", 3)}, 10000, nil, 72 * time.Hour, 3 * time.Hour, "expiry"},
		{"no history later stock", []model.OrderBatch{batch("20", 3), batch("10", 100)}, 10000, nil, 72 * time.Hour, -1, "unknown"},
		{"exact demand horizon", []model.OrderBatch{batch("82", 100)}, 10000, rate, 72 * time.Hour, -1, "unknown"},
		{"expiry at horizon", []model.OrderBatch{batch("100", 72)}, 10000, nil, 72 * time.Hour, 72 * time.Hour, "expiry"},
		{"expired excluded", []model.OrderBatch{batch("100", 0)}, 10000, nil, 72 * time.Hour, 0, "already_unavailable"},
		{"same expiry", []model.OrderBatch{batch("6", 2), batch("6", 2), batch("8", 100)}, 10000, nil, 72 * time.Hour, 2 * time.Hour, "expiry"},
		{"exactly one portion", []model.OrderBatch{batch("10", 100)}, 10000, rate, 72 * time.Hour, 1, "estimated_demand"},
		{"milli portion", []model.OrderBatch{batch("0.003", 100)}, 1, rate, 72 * time.Hour, 7200000001, "estimated_demand"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]model.OrderBatch{}, tc.batches...)
			at, cause, err := ingredientStop(tc.batches, big.NewInt(tc.need), tc.rate, now, tc.horizon)
			if err != nil || cause != tc.cause || tc.at < 0 && at != nil || tc.at >= 0 && (at == nil || !at.Equal(now.Add(tc.at))) {
				t.Fatal(at, cause, err, "want", tc.at, tc.cause)
			}
			if !reflect.DeepEqual(before, tc.batches) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestIngredientSimulationHugeAndFractionalQuantities(t *testing.T) {
	now := time.Now().UTC()
	// No float rounding, int64 quantity overflow or rounded intermediate event.
	batches := []model.OrderBatch{{RemainingQuantity: "99999999999999999.999", ExpiresAt: now.Add(100 * time.Hour)}}
	required, _ := new(big.Int).SetString("99999999999999999998", 10)
	rate := new(big.Rat).SetFrac(big.NewInt(3), big.NewInt(2))
	at, cause, err := ingredientStop(batches, required, rate, now, time.Hour)
	if err != nil || cause != "estimated_demand" || !at.Equal(now.Add(1)) {
		t.Fatal(at, cause, err)
	}
	// The first batch depletes at 2/3 ns. Rounding that intermediate event
	// would change the result after consumption continues in the next batch.
	batches = []model.OrderBatch{
		{RemainingQuantity: "0.001", ExpiresAt: now.Add(time.Hour)},
		{RemainingQuantity: "0.003", ExpiresAt: now.Add(2 * time.Hour)},
	}
	at, cause, err = ingredientStop(batches, big.NewInt(1), rate, now, time.Hour)
	if err != nil || cause != "estimated_demand" || at == nil || !at.Equal(now.Add(3)) {
		t.Fatal(at, cause, err)
	}
}
