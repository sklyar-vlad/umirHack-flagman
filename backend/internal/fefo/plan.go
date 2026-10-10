// Package fefo plans exact stock allocation without database writes. Orders and
// recommendation simulations can use the same expiry and tie-breaking rules.
package fefo

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
)

var ErrInsufficientStock = errors.New("insufficient stock for FEFO allocation")

type Demand struct {
	ItemID, ProductID uuid.UUID
	Quantity          string
}

type Allocation struct {
	ItemID                           uuid.UUID
	Batch                            model.OrderBatch
	Quantity, RemainingQuantityAfter string
}

// Plan preserves demand order and uses expiry, receipt time, then batch ID. A batch
// shared by multiple items has one running balance. Insufficient stock returns
// no partial plan. Neither input slice is mutated; now is fixed by the caller.
func Plan(demands []Demand, batches []model.OrderBatch, now time.Time) ([]Allocation, error) {
	ordered := slices.Clone(batches)
	slices.SortFunc(ordered, func(a, b model.OrderBatch) int {
		if order := a.ExpiresAt.Compare(b.ExpiresAt); order != 0 {
			return order
		}
		if order := a.ReceivedAt.Compare(b.ReceivedAt); order != 0 {
			return order
		}
		return slices.Compare(a.ID[:], b.ID[:])
	})
	remaining := make(map[uuid.UUID]*big.Int, len(ordered))
	byProduct := make(map[uuid.UUID][]model.OrderBatch)
	for _, b := range ordered {
		if b.ID == uuid.Nil || b.ProductID == uuid.Nil {
			return nil, fmt.Errorf("invalid FEFO batch identity")
		}
		if _, exists := remaining[b.ID]; exists {
			return nil, fmt.Errorf("duplicate FEFO batch %s", b.ID)
		}
		value, err := quantity.ParseMilli(b.RemainingQuantity)
		if err != nil || value.Sign() < 0 {
			return nil, fmt.Errorf("invalid FEFO batch quantity for %s", b.ID)
		}
		remaining[b.ID] = value
		if value.Sign() > 0 && b.ExpiresAt.After(now) {
			byProduct[b.ProductID] = append(byProduct[b.ProductID], b)
		}
	}
	plan := []Allocation{}
	for _, demand := range demands {
		need, err := quantity.ParseMilli(demand.Quantity)
		if err != nil || need.Sign() <= 0 || demand.ItemID == uuid.Nil || demand.ProductID == uuid.Nil {
			return nil, fmt.Errorf("invalid FEFO demand")
		}
		for _, b := range byProduct[demand.ProductID] {
			balance := remaining[b.ID]
			if balance.Sign() == 0 {
				continue
			}
			used := new(big.Int).Set(need)
			if used.Cmp(balance) > 0 {
				used.Set(balance)
			}
			balance.Sub(balance, used)
			need.Sub(need, used)
			plan = append(plan, Allocation{ItemID: demand.ItemID, Batch: b, Quantity: quantity.FormatMilli(used), RemainingQuantityAfter: quantity.FormatMilli(balance)})
			if need.Sign() == 0 {
				break
			}
		}
		if need.Sign() > 0 {
			return nil, fmt.Errorf("%w: product %s missing %s", ErrInsufficientStock, demand.ProductID, quantity.FormatMilli(need))
		}
	}
	return plan, nil
}
