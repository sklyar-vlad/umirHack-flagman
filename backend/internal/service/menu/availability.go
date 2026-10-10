package menu

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
)

// CalculateAvailability uses exact quantities in thousandths. Aggregated stock
// may exceed one NUMERIC(14,3) batch, so it must not use float64 or int64 sums.
func CalculateAvailability(active bool, recipe []model.RecipeIngredient) (dto.Availability, error) {
	result := dto.Availability{AvailablePortions: "0", Shortages: []dto.Shortage{}, UnavailabilityReasons: []string{}}
	if !active {
		result.UnavailabilityReasons = append(result.UnavailabilityReasons, "dish_inactive")
		return result, nil
	}
	if len(recipe) == 0 {
		result.UnavailabilityReasons = append(result.UnavailabilityReasons, "recipe_missing")
		return result, nil
	}
	var minimum *big.Int
	for _, ingredient := range recipe {
		required, err := quantity.ParseMilli(ingredient.QuantityPerPortion)
		if err != nil || required.Sign() <= 0 {
			return dto.Availability{}, fmt.Errorf("invalid recipe quantity for %s", ingredient.ProductID)
		}
		available, err := quantity.ParseMilli(ingredient.AvailableQuantity)
		if err != nil {
			return dto.Availability{}, err
		}
		expired, err := quantity.ParseMilli(ingredient.ExpiredQuantity)
		if err != nil {
			return dto.Availability{}, err
		}
		portions := new(big.Int).Quo(available, required)
		if minimum == nil || portions.Cmp(minimum) < 0 {
			minimum = portions
		}
		if available.Cmp(required) >= 0 {
			continue
		}
		reason := "ingredient_shortage"
		if available.Sign() == 0 && expired.Sign() > 0 {
			reason = "only_expired_stock"
		}
		result.Shortages = append(result.Shortages, dto.Shortage{
			ProductID: ingredient.ProductID, ProductName: ingredient.ProductName, Unit: ingredient.Unit,
			RequiredQuantity: quantity.FormatMilli(required), AvailableQuantity: quantity.FormatMilli(available),
			MissingQuantity: quantity.FormatMilli(new(big.Int).Sub(required, available)),
			ExpiredQuantity: quantity.FormatMilli(expired), ReasonCode: reason,
		})
		if !slices.Contains(result.UnavailabilityReasons, reason) {
			result.UnavailabilityReasons = append(result.UnavailabilityReasons, reason)
		}
	}
	result.AvailablePortions = minimum.String()
	result.IsAvailable = minimum.Sign() > 0
	return result, nil
}
