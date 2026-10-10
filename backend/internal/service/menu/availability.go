package menu

import (
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
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
		required, err := scaledQuantity(ingredient.QuantityPerPortion)
		if err != nil || required.Sign() <= 0 {
			return dto.Availability{}, fmt.Errorf("invalid recipe quantity for %s", ingredient.ProductID)
		}
		available, err := scaledQuantity(ingredient.AvailableQuantity)
		if err != nil {
			return dto.Availability{}, err
		}
		expired, err := scaledQuantity(ingredient.ExpiredQuantity)
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
			RequiredQuantity: decimalQuantity(required), AvailableQuantity: decimalQuantity(available),
			MissingQuantity: decimalQuantity(new(big.Int).Sub(required, available)),
			ExpiredQuantity: decimalQuantity(expired), ReasonCode: reason,
		})
		if !slices.Contains(result.UnavailabilityReasons, reason) {
			result.UnavailabilityReasons = append(result.UnavailabilityReasons, reason)
		}
	}
	result.AvailablePortions = minimum.String()
	result.IsAvailable = minimum.Sign() > 0
	return result, nil
}

func scaledQuantity(raw string) (*big.Int, error) {
	whole, fraction, hasFraction := strings.Cut(raw, ".")
	if whole == "" || len(fraction) > 3 || (hasFraction && fraction == "") {
		return nil, fmt.Errorf("invalid stock quantity %q", raw)
	}
	for _, digit := range whole + fraction {
		if digit < '0' || digit > '9' {
			return nil, fmt.Errorf("invalid stock quantity %q", raw)
		}
	}
	value, ok := new(big.Int).SetString(whole+fraction+strings.Repeat("0", 3-len(fraction)), 10)
	if !ok {
		return nil, fmt.Errorf("invalid stock quantity %q", raw)
	}
	return value, nil
}

func decimalQuantity(value *big.Int) string {
	digits := value.String()
	if len(digits) <= 3 {
		digits = strings.Repeat("0", 4-len(digits)) + digits
	}
	return digits[:len(digits)-3] + "." + digits[len(digits)-3:]
}
