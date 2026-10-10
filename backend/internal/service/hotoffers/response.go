package hotoffers

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	batchservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/batch"
)

func renderOffer(offer *scoredOffer, rank int64, now time.Time) dto.HotOffer {
	item := dto.HotOffer{
		Dish: dto.HotOfferDish{
			ID:            offer.dish.ID,
			Name:          offer.dish.Name,
			RecipeVersion: *offer.dish.RecipeVersion,
		},
		Rank:              rank,
		PriorityScore:     offer.score.FloatString(4),
		PriorityLevel:     offer.level,
		AvailablePortions: offer.availablePortions,
		ProblemBatches:    []dto.HotOfferBatch{},
	}
	problems := slices.Clone(offer.problems)
	slices.SortFunc(problems, func(a, b problemBatch) int {
		if order := a.allocation.Batch.ExpiresAt.Compare(b.allocation.Batch.ExpiresAt); order != 0 {
			return order
		}
		if order := a.allocation.Batch.ReceivedAt.Compare(b.allocation.Batch.ReceivedAt); order != 0 {
			return order
		}
		return slices.Compare(a.allocation.Batch.ID[:], b.allocation.Batch.ID[:])
	})
	reasons := make([]string, 0, len(problems))
	for i := range problems {
		problem := &problems[i]
		a := problem.allocation
		b := a.Batch
		seconds := batchservice.RemainingSeconds(b.ExpiresAt, now)
		item.ProblemBatches = append(item.ProblemBatches, dto.HotOfferBatch{
			BatchID:           b.ID,
			ProductID:         b.ProductID,
			ProductName:       problem.ingredient.ProductName,
			SupplyID:          b.SupplyID,
			SupplyNumber:      b.SupplyNumber,
			LineNumber:        b.LineNumber,
			Unit:              problem.ingredient.Unit,
			RemainingQuantity: b.RemainingQuantity,
			UsedForOnePortion: a.Quantity,
			ExpiresAt:         b.ExpiresAt.UTC(),
			RemainingSeconds:  seconds,
		})
		unit := map[string]string{"g": "г", "ml": "мл", "pcs": "шт"}[problem.ingredient.Unit]
		used := strings.TrimRight(strings.TrimRight(a.Quantity, "0"), ".")
		reasons = append(
			reasons,
			fmt.Sprintf(
				"%s из поставки №%s, строка %d, истечёт %s; порция использует %s %s из этой партии",
				problem.ingredient.ProductName,
				b.SupplyNumber,
				b.LineNumber,
				expiryText(seconds),
				used,
				unit,
			),
		)
	}
	item.Reason = strings.Join(reasons, "; ")
	return item
}

func expiryText(seconds int64) string {
	if seconds == 0 {
		return "менее чем через секунду"
	}
	parts := []string{}
	if hours := seconds / 3600; hours > 0 {
		parts = append(parts, fmt.Sprintf("%d ч", hours))
	}
	if minutes := seconds % 3600 / 60; minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d мин", minutes))
	}
	if rest := seconds % 60; rest > 0 {
		parts = append(parts, fmt.Sprintf("%d с", rest))
	}
	return "через " + strings.Join(parts, " ")
}
