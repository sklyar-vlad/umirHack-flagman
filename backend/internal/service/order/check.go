package order

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
)

type ingredientDemand struct {
	ingredient model.RecipeIngredient
	required   *big.Int
}

func checkBasket(items []basketItem, dishes map[uuid.UUID]model.Dish) (dto.OrderCheck, error) {
	result := dto.OrderCheck{
		Items: []dto.OrderItemSnapshot{}, Requirements: []dto.Requirement{},
		Shortages: []dto.Shortage{}, Issues: []dto.OrderIssue{}, Meta: map[string]any{},
	}
	demands := make(map[uuid.UUID]*ingredientDemand)
	for _, item := range items {
		dish := dishes[item.dishID]
		result.Items = append(result.Items, dto.OrderItemSnapshot{
			DishID: dish.ID, DishName: dish.Name, Portions: item.portions, RecipeVersion: dish.RecipeVersion,
		})
		if !dish.IsActive {
			result.Issues = append(result.Issues, dto.OrderIssue{
				DishID: dish.ID, Code: "dish_inactive", Message: "Блюдо отключено", CurrentRecipeVersion: dish.RecipeVersion,
			})
		}
		if dish.RecipeVersion == nil || len(dish.Recipe) == 0 {
			result.Issues = append(result.Issues, dto.OrderIssue{
				DishID: dish.ID, Code: "recipe_missing", Message: "У блюда нет рецептуры", CurrentRecipeVersion: dish.RecipeVersion,
			})
		}
		if item.recipeVersion != nil && dish.RecipeVersion != nil && *item.recipeVersion != *dish.RecipeVersion {
			result.Issues = append(result.Issues, dto.OrderIssue{
				DishID: dish.ID, Code: "recipe_changed", Message: "Рецептура блюда изменилась", CurrentRecipeVersion: dish.RecipeVersion,
			})
		}
		for _, ingredient := range dish.Recipe {
			perPortion, err := quantity.ParseMilli(ingredient.QuantityPerPortion)
			if err != nil || perPortion.Sign() <= 0 {
				return dto.OrderCheck{}, fmt.Errorf("invalid recipe quantity for dish %s product %s", dish.ID, ingredient.ProductID)
			}
			demand, exists := demands[ingredient.ProductID]
			if !exists {
				demand = &ingredientDemand{ingredient: ingredient, required: new(big.Int)}
				demands[ingredient.ProductID] = demand
			}
			demand.required.Add(demand.required, perPortion.Mul(perPortion, big.NewInt(int64(item.portions))))
		}
	}
	ordered := make([]*ingredientDemand, 0, len(demands))
	for _, demand := range demands {
		ordered = append(ordered, demand)
	}
	slices.SortFunc(ordered, func(a, b *ingredientDemand) int {
		if name := cmp.Compare(a.ingredient.ProductName, b.ingredient.ProductName); name != 0 {
			return name
		}
		return cmp.Compare(a.ingredient.ProductID.String(), b.ingredient.ProductID.String())
	})
	for _, demand := range ordered {
		ingredient := demand.ingredient
		available, err := quantity.ParseMilli(ingredient.AvailableQuantity)
		if err != nil {
			return dto.OrderCheck{}, fmt.Errorf("stock for product %s: %w", ingredient.ProductID, err)
		}
		expired, err := quantity.ParseMilli(ingredient.ExpiredQuantity)
		if err != nil {
			return dto.OrderCheck{}, fmt.Errorf("expired stock for product %s: %w", ingredient.ProductID, err)
		}
		missing := new(big.Int).Sub(demand.required, available)
		if missing.Sign() < 0 {
			missing.SetInt64(0)
		}
		requirement := dto.Requirement{
			ProductID: ingredient.ProductID, ProductName: ingredient.ProductName, Unit: ingredient.Unit,
			RequiredQuantity: quantity.FormatMilli(demand.required), AvailableQuantity: quantity.FormatMilli(available),
			MissingQuantity: quantity.FormatMilli(missing),
		}
		result.Requirements = append(result.Requirements, requirement)
		if missing.Sign() == 0 {
			continue
		}
		reason := "ingredient_shortage"
		if available.Sign() == 0 && expired.Sign() > 0 {
			reason = "only_expired_stock"
		}
		result.Shortages = append(result.Shortages, dto.Shortage{
			ProductID: requirement.ProductID, ProductName: requirement.ProductName, Unit: requirement.Unit,
			RequiredQuantity: requirement.RequiredQuantity, AvailableQuantity: requirement.AvailableQuantity,
			MissingQuantity: requirement.MissingQuantity, ExpiredQuantity: quantity.FormatMilli(expired), ReasonCode: reason,
		})
	}
	result.CanCreate = len(result.Issues) == 0 && len(result.Shortages) == 0
	return result, nil
}
