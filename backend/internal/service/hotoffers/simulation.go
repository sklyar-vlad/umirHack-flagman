package hotoffers

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/fefo"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
)

type problemBatch struct {
	allocation fefo.Allocation
	ingredient model.RecipeIngredient
}

type simulatedOffer struct {
	dish              model.Dish
	availablePortions string
	problems          []problemBatch
}

// simulate starts each dish from the same stock. Recommendations are alternatives,
// not a combined basket; the shared FEFO planner never mutates its inputs.
func simulate(dishes []model.Dish, batches []model.OrderBatch, now time.Time, hours int) ([]simulatedOffer, error) {
	if hours < 1 || hours > 720 {
		return nil, fmt.Errorf("invalid hot offers threshold")
	}
	warningAt := now.Add(time.Duration(hours) * time.Hour)
	offers := []simulatedOffer{}
	for _, dish := range dishes {
		availability, err := menu.CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return nil, err
		}
		if !availability.IsAvailable || dish.RecipeVersion == nil {
			continue
		}
		demands := make([]fefo.Demand, 0, len(dish.Recipe))
		ingredients := make(map[uuid.UUID]model.RecipeIngredient, len(dish.Recipe))
		for _, ingredient := range dish.Recipe {
			demands = append(
				demands,
				fefo.Demand{ItemID: dish.ID, ProductID: ingredient.ProductID, Quantity: ingredient.QuantityPerPortion},
			)
			ingredients[ingredient.ProductID] = ingredient
		}
		plan, err := fefo.Plan(demands, batches, now)
		if err != nil {
			return nil, fmt.Errorf("simulate hot offer %s: %w", dish.ID, err)
		}
		offer := simulatedOffer{
			dish:              dish,
			availablePortions: availability.AvailablePortions,
			problems:          []problemBatch{},
		}
		for i := range plan {
			allocation := &plan[i]
			if !allocation.Batch.ExpiresAt.After(warningAt) {
				offer.problems = append(
					offer.problems,
					problemBatch{allocation: *allocation, ingredient: ingredients[allocation.Batch.ProductID]},
				)
			}
		}
		if len(offer.problems) > 0 {
			offers = append(offers, offer)
		}
	}
	return offers, nil
}
