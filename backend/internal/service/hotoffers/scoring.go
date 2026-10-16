package hotoffers

import (
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
)

type scoredOffer struct {
	simulatedOffer
	score   *big.Rat
	nearest time.Time
	level   string
}

func scoreOffers(offers []simulatedOffer, now time.Time, hours int) ([]scoredOffer, error) {
	if hours < 1 || hours > 720 {
		return nil, fmt.Errorf("invalid hot offers threshold")
	}
	horizon := time.Duration(hours) * time.Hour
	scored := make([]scoredOffer, 0, len(offers))
	for i := range offers {
		offer := &offers[i]
		if len(offer.problems) == 0 {
			return nil, fmt.Errorf("hot offer has no consumed expiring batches")
		}
		item := scoredOffer{
			simulatedOffer: *offer,
			score:          new(big.Rat),
			nearest:        offer.problems[0].allocation.Batch.ExpiresAt,
		}
		for j := range offer.problems {
			problem := &offer.problems[j]
			allocation := problem.allocation
			remaining, err := quantity.ParseMilli(allocation.Batch.RemainingQuantity)
			if err != nil || remaining.Sign() <= 0 {
				return nil, fmt.Errorf("invalid hot offer batch remaining quantity")
			}
			used, err := quantity.ParseMilli(allocation.Quantity)
			if err != nil || used.Sign() <= 0 || used.Cmp(remaining) > 0 {
				return nil, fmt.Errorf("invalid hot offer batch usage")
			}
			h := allocation.Batch.ExpiresAt.Sub(now)
			if h <= 0 || h > horizon {
				return nil, fmt.Errorf("hot offer batch is outside expiring threshold")
			}
			score := big.NewRat(int64(horizon-h), int64(horizon))
			score.Mul(score, new(big.Rat).SetFrac(used, remaining))
			score.Mul(score, big.NewRat(100, 1))
			if score.Cmp(item.score) > 0 {
				item.score = score
			}
			if allocation.Batch.ExpiresAt.Before(item.nearest) {
				item.nearest = allocation.Batch.ExpiresAt
			}
		}
		switch h := item.nearest.Sub(now); {
		case h <= horizon/4:
			item.level = "high"
		case h <= horizon/2:
			item.level = "medium"
		default:
			item.level = "low"
		}
		scored = append(scored, item)
	}
	slices.SortFunc(scored, func(a, b scoredOffer) int {
		if order := a.score.Cmp(b.score); order != 0 {
			return -order
		}
		if order := a.nearest.Compare(b.nearest); order != 0 {
			return order
		}
		return slices.Compare(a.dish.ID[:], b.dish.ID[:])
	})
	return scored, nil
}
