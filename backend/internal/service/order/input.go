package order

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type basketItem struct {
	dishID        uuid.UUID
	portions      int
	recipeVersion *int
	inputIndex    int
}

// normalizeBasket preserves first-occurrence order and validates merged portions
// before any repository reads. An omitted version accepts the explicit version
// of another occurrence; conflicting explicit versions are invalid input.
func normalizeBasket(request dto.OrderRequest) ([]basketItem, error) {
	if err := validations.ValidateCount("items", len(request.Items)); err != nil {
		return nil, err
	}
	items := make([]basketItem, 0, len(request.Items))
	positions := make(map[uuid.UUID]int, len(request.Items))
	for i, item := range request.Items {
		field := fmt.Sprintf("items[%d]", i)
		id, err := validations.ParseUUID(field+".dish_id", item.DishID)
		if err != nil {
			return nil, err
		}
		if id == uuid.Nil {
			return nil, &validations.FieldError{Field: field + ".dish_id", Message: "Нулевой UUID блюда недопустим"}
		}
		if err := validations.ValidatePortions(field+".portions", item.Portions); err != nil {
			return nil, err
		}
		if item.RecipeVersion != nil && *item.RecipeVersion < 1 {
			return nil, &validations.FieldError{
				Field:   field + ".recipe_version",
				Message: "Версия рецепта должна быть положительным целым числом",
			}
		}
		if position, exists := positions[id]; exists {
			previous := &items[position]
			if previous.recipeVersion != nil && item.RecipeVersion != nil &&
				*previous.recipeVersion != *item.RecipeVersion {
				return nil, &validations.FieldError{
					Field:   field + ".recipe_version",
					Message: "Повторы блюда должны иметь одинаковую версию рецепта",
				}
			}
			if err := validations.ValidatePortions(field+".portions", previous.portions+item.Portions); err != nil {
				return nil, err
			}
			previous.portions += item.Portions
			if previous.recipeVersion == nil {
				previous.recipeVersion = item.RecipeVersion
			}
			continue
		}
		positions[id] = len(items)
		items = append(
			items,
			basketItem{dishID: id, portions: item.Portions, recipeVersion: item.RecipeVersion, inputIndex: i},
		)
	}
	return items, nil
}
