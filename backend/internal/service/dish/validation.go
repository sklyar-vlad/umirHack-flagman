package dish

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func field(name string) error {
	return &validations.FieldError{Field: name, Message: "Некорректное значение"}
}

func validateDish(d *model.Dish) error {
	if !utf8.ValidString(d.Name) {
		return field("name")
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > 200 {
		return field("name")
	}
	for _, r := range d.Name {
		if unicode.IsControl(r) {
			return field("name")
		}
	}
	if d.Description != nil &&
		(!utf8.ValidString(*d.Description) || utf8.RuneCountInString(*d.Description) > 2000 || strings.ContainsRune(*d.Description, 0)) {
		return field("description")
	}
	return nil
}

func validateRecipe(input []dto.RecipeInput) ([]model.RecipeIngredient, error) {
	if err := validations.ValidateCount("recipe", len(input)); err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]bool, len(input))
	result := make([]model.RecipeIngredient, 0, len(input))
	for i, item := range input {
		id, err := validations.ParseUUID(fmt.Sprintf("recipe[%d].product_id", i), item.ProductID)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, apperrors.ErrDuplicateIngredient
		}
		seen[id] = true
		value, err := validations.ParseQuantity(
			fmt.Sprintf("recipe[%d].quantity_per_portion", i),
			item.QuantityPerPortion,
		)
		if err != nil {
			return nil, err
		}
		if value <= 0 {
			return nil, field(fmt.Sprintf("recipe[%d].quantity_per_portion", i))
		}
		result = append(
			result,
			model.RecipeIngredient{ProductID: id, QuantityPerPortion: fmt.Sprintf("%d.%03d", value/1000, value%1000)},
		)
	}
	return result, nil
}

func wholeQuantity(value string) bool {
	_, fraction, _ := strings.Cut(value, ".")
	return strings.Trim(fraction, "0") == ""
}
