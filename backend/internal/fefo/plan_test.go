package fefo

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestPlanExpiryTieBreakSharedBalanceAndExactDecimals(t *testing.T) {
	now := time.Now().UTC()
	product, other, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	low := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	high := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	batches := []model.OrderBatch{
		{ID: high, ProductID: product, RemainingQuantity: "0.300", ExpiresAt: now.Add(time.Hour)},
		{ID: uuid.New(), ProductID: product, RemainingQuantity: "100.000", ExpiresAt: now},
		{ID: low, ProductID: product, RemainingQuantity: "0.100", ExpiresAt: now.Add(time.Hour)},
		{ID: uuid.New(), ProductID: other, RemainingQuantity: "0.010", ExpiresAt: now.Add(time.Hour)},
		{ID: uuid.New(), ProductID: product, RemainingQuantity: "0.000", ExpiresAt: now.Add(-time.Hour)},
	}
	before := append([]model.OrderBatch(nil), batches...)
	demands := []Demand{{first, product, "0.200"}, {second, product, "0.150"}, {second, other, "0.001"}}
	plan, err := Plan(demands, batches, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 4 || plan[0].Batch.ID != low || plan[0].Quantity != "0.100" || plan[1].Batch.ID != high || plan[1].RemainingQuantityAfter != "0.200" || plan[2].ItemID != second || plan[2].Quantity != "0.150" || plan[2].RemainingQuantityAfter != "0.050" || plan[3].RemainingQuantityAfter != "0.009" {
		t.Fatalf("plan: %+v", plan)
	}
	if !reflect.DeepEqual(before, batches) {
		t.Fatal("planner mutated stock input")
	}
	again, err := Plan(demands, batches, now)
	if err != nil || !reflect.DeepEqual(plan, again) {
		t.Fatal("simulation is not repeatable")
	}
}

func TestPlanLargeDemandAndNoPartialResult(t *testing.T) {
	now := time.Now()
	product, item := uuid.New(), uuid.New()
	batches := []model.OrderBatch{{ID: uuid.New(), ProductID: product, RemainingQuantity: "99999999999.999", ExpiresAt: now.Add(time.Hour)}, {ID: uuid.New(), ProductID: product, RemainingQuantity: "99999999999.999", ExpiresAt: now.Add(2 * time.Hour)}}
	plan, err := Plan([]Demand{{item, product, "199999999999.998"}}, batches, now)
	if err != nil || len(plan) != 2 || plan[1].RemainingQuantityAfter != "0.000" {
		t.Fatalf("large allocation: %+v %v", plan, err)
	}
	plan, err = Plan([]Demand{{item, product, "199999999999.999"}}, batches, now)
	if !errors.Is(err, ErrInsufficientStock) || plan != nil {
		t.Fatalf("partial shortage plan: %+v %v", plan, err)
	}
}

func TestPlanRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	product, item := uuid.New(), uuid.New()
	b := model.OrderBatch{ID: uuid.New(), ProductID: product, RemainingQuantity: "1.000", ExpiresAt: now.Add(time.Hour)}
	for _, value := range []string{"0.000", "-1.000", "1.0001", "bad"} {
		if _, err := Plan([]Demand{{item, product, value}}, []model.OrderBatch{b}, now); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if _, err := Plan(nil, []model.OrderBatch{b, b}, now); err == nil {
		t.Fatal("accepted duplicate batch")
	}
	b.RemainingQuantity = "-1.000"
	if _, err := Plan(nil, []model.OrderBatch{b}, now); err == nil {
		t.Fatal("accepted negative stock")
	}
}

func TestPlanReceiptTimePrecedesBatchID(t *testing.T) {
	now := time.Now()
	product, item := uuid.New(), uuid.New()
	laterID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	earlierID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	batches := []model.OrderBatch{
		{ID: laterID, ProductID: product, RemainingQuantity: "1.000", ExpiresAt: now.Add(time.Hour), ReceivedAt: now},
		{ID: earlierID, ProductID: product, RemainingQuantity: "1.000", ExpiresAt: now.Add(time.Hour), ReceivedAt: now.Add(-time.Hour)},
	}
	plan, err := Plan([]Demand{{item, product, "1.000"}}, batches, now)
	if err != nil || len(plan) != 1 || plan[0].Batch.ID != earlierID {
		t.Fatalf("receipt ordering: %+v %v", plan, err)
	}
}
