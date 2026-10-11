package forecast

import (
	"fmt"
	"math/big"
	"slices"
	"sort"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/fefo"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
)

type simulatedBatch struct {
	remaining *big.Rat
	expires   *big.Rat // nanoseconds relative to the snapshot time
}

// ingredientStop simulates continuous average warehouse consumption, independent
// of the candidate dish. Missing history uses an explicit expiry-only scenario.
// Every quantity and event offset stays rational until the returned timestamp.
func ingredientStop(
	batches []model.OrderBatch,
	required *big.Int,
	rate *big.Rat,
	now time.Time,
	horizon time.Duration,
) (*time.Time, string, error) {
	ordered := slices.Clone(batches)
	sort.Slice(ordered, func(i, j int) bool { return fefo.CompareBatches(&ordered[i], &ordered[j]) < 0 })
	stock := []simulatedBatch{}
	total := new(big.Rat)
	for i := range ordered {
		b := &ordered[i]
		n, err := quantity.ParseMilli(b.RemainingQuantity)
		if err != nil {
			return nil, "", fmt.Errorf("forecast batch: %w", err)
		}
		if n.Sign() == 0 || !b.ExpiresAt.After(now) {
			continue
		}
		v := new(big.Rat).SetInt(n)
		total.Add(total, v)
		stock = append(stock, simulatedBatch{remaining: v, expires: new(big.Rat).SetInt64(int64(b.ExpiresAt.Sub(now)))})
	}
	need := new(big.Rat).SetInt(required)
	if total.Cmp(need) < 0 {
		at := now.UTC()
		return &at, "already_unavailable", nil
	}
	if rate == nil {
		rate = new(big.Rat)
	}
	if rate.Sign() < 0 || required.Sign() <= 0 {
		return nil, "", fmt.Errorf("invalid forecast requirement or rate")
	}
	cursor := new(big.Rat)
	end := new(big.Rat).SetInt64(int64(horizon))
	for i := 0; i < len(stock); i++ {
		b := &stock[i]
		// The next event is either depletion of the FEFO head or its expiry.
		next := new(big.Rat).Set(b.expires)
		expiry := true
		if rate.Sign() > 0 {
			depletion := new(big.Rat).Add(cursor, new(big.Rat).Quo(b.remaining, rate))
			if depletion.Cmp(next) < 0 {
				next = depletion
				expiry = false
			}
			threshold := new(big.Rat).Add(cursor, new(big.Rat).Quo(new(big.Rat).Sub(total, need), rate))
			// Equality still cooks one portion. Return the first nanosecond
			// strictly beyond that threshold, only if it is inside the horizon.
			demandNS := new(big.Int).Quo(threshold.Num(), threshold.Denom())
			demandNS.Add(demandNS, big.NewInt(1))
			if threshold.Cmp(next) < 0 && demandNS.Cmp(big.NewInt(int64(horizon))) <= 0 {
				at := now.UTC().Add(time.Duration(demandNS.Int64()))
				return &at, "estimated_demand", nil
			}
		}
		if next.Cmp(end) > 0 {
			return nil, causeUnknown, nil
		}
		used := new(big.Rat).Mul(rate, new(big.Rat).Sub(next, cursor))
		total.Sub(total, used)
		b.remaining.Sub(b.remaining, used)
		if expiry {
			total.Sub(total, b.remaining)
		}
		cursor.Set(next)
		if total.Cmp(need) < 0 {
			// Expiry events are integral nanoseconds. A pure depletion cannot
			// skip the one-portion threshold handled above.
			at := now.UTC().Add(time.Duration(new(big.Int).Quo(next.Num(), next.Denom()).Int64()))
			return &at, "expiry", nil
		}
	}
	return nil, causeUnknown, nil
}
