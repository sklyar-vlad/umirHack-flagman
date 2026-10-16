package forecast

import (
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestRatesFullLookbackExactAndUnknown(t *testing.T) {
	id := uuid.New()
	rates, err := consumptionRates(
		[]model.ConsumptionTotal{
			{ProductID: id, Unit: "g", Quantity: "0.001"},
			{ProductID: id, Unit: "g", Quantity: "0.002"},
			{ProductID: id, Unit: "pcs", Quantity: "7"},
		},
		7,
	)
	if err != nil {
		t.Fatal(err)
	}
	if rates[productUnit{id, "g"}].Cmp(new(big.Rat).SetFrac(big.NewInt(3), big.NewInt(int64(7*24*time.Hour)))) != 0 {
		t.Fatal(rates)
	}
	if rates[productUnit{id, "ml"}] != nil {
		t.Fatal("incompatible units mixed")
	}
	empty, err := consumptionRates([]model.ConsumptionTotal{{ProductID: id, Unit: "g", Quantity: "0.000"}}, 7)
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	if _, err := consumptionRates(nil, 0); err == nil {
		t.Fatal("invalid lookback")
	}
	for _, f := range []model.ForecastFilter{{LookbackDays: 0, HorizonHours: 72, Page: 1, PageSize: 20}, {LookbackDays: 91, HorizonHours: 72, Page: 1, PageSize: 20}, {LookbackDays: 7, HorizonHours: 721, Page: 1, PageSize: 20}, {LookbackDays: 7, HorizonHours: 0, Page: 1, PageSize: 20}, {LookbackDays: 7, HorizonHours: 72, Page: 0, PageSize: 20}} {
		if validateFilter(f) == nil {
			t.Fatal(f)
		}
	}
}
