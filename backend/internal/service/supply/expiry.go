package supply

import (
	"fmt"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

// parseExpiryInput validates the shape and dates without consulting the clock or products.
func parseExpiryInput(
	path string,
	item dto.CreateSupplyItem,
	receivedAt time.Time,
) (model.BatchExpiry, error) {
	var result model.BatchExpiry

	if item.ProducedAt != nil {
		producedAt, err := parseBatchDate(
			path+".produced_at",
			*item.ProducedAt,
		)
		if err != nil {
			return result, err
		}

		if producedAt.After(receivedAt) {
			return result, &validations.FieldError{
				Field:   path + ".produced_at",
				Message: "Дата производства не может быть позже приёмки",
			}
		}

		result.ProducedAt = &producedAt
	}

	if item.ExpiresAt != nil {
		if item.ShelfLifeHours != nil {
			return result, &validations.FieldError{
				Field:   path + ".shelf_life_hours",
				Message: "При явном expires_at срок в часах не передаётся",
			}
		}

		expiresAt, err := parseBatchDate(
			path+".expires_at",
			*item.ExpiresAt,
		)
		if err != nil {
			return result, err
		}

		result.Source = model.ExpiryExplicit
		result.ExpiresAt = expiresAt
		if !expiresAt.After(receivedAt) {
			return result, &validations.FieldError{
				Field: path + ".expires_at", Message: "Срок годности должен быть позже даты приёмки",
			}
		}
	} else {
		if result.ProducedAt == nil {
			return result, &validations.FieldError{
				Field: path + ".produced_at",
				Message: "Передайте expires_at либо дату " +
					"производства для расчёта",
			}
		}

		result.Source = model.ExpiryCalculated
		if item.ShelfLifeHours != nil && (*item.ShelfLifeHours <= 0 || int64(*item.ShelfLifeHours) > 2147483647) {
			return result, &validations.FieldError{
				Field: path + ".shelf_life_hours", Message: "Срок должен быть целым числом от 1 до 2147483647 часов",
			}
		}
	}
	return result, nil
}

func resolveExpiry(
	path string,
	item dto.CreateSupplyItem,
	product model.SupplyProduct,
	receivedAt time.Time,
	serverTime time.Time,
) (model.BatchExpiry, error) {
	result, err := parseExpiryInput(path, item, receivedAt)
	if err != nil {
		return result, err
	}
	if result.Source == model.ExpiryCalculated {
		hours := item.ShelfLifeHours
		if hours == nil {
			hours = product.StandardShelfLifeHours
		}

		if hours == nil {
			return result, fmt.Errorf(
				"%w: %w",
				apperrors.ErrShelfLifeRequired,
				&validations.FieldError{
					Field: path + ".shelf_life_hours",
					Message: "Укажите срок в часах: " +
						"у продукта нет стандартного срока",
				},
			)
		}

		if *hours <= 0 || int64(*hours) > 2147483647 {
			return result, &validations.FieldError{
				Field: path + ".shelf_life_hours",
				Message: "Срок должен быть целым числом " +
					"от 1 до 2147483647 часов",
			}
		}

		actualHours := *hours

		// В UTC сутки равны 24 часам.
		// Деление на дни предотвращает переполнение time.Duration.
		expiresAt := result.ProducedAt.
			AddDate(0, 0, actualHours/24).
			Add(time.Duration(actualHours%24) * time.Hour)

		if expiresAt.Year() > 9999 {
			return result, &validations.FieldError{
				Field: path + ".shelf_life_hours",
				Message: "Рассчитанная дата выходит " +
					"за диапазон RFC3339",
			}
		}

		result.Source = model.ExpiryCalculated
		result.ShelfLifeHours = &actualHours
		result.ExpiresAt = expiresAt
	}

	if !result.ExpiresAt.After(receivedAt) {
		return result, &validations.FieldError{
			Field:   path + ".expires_at",
			Message: "Срок годности должен быть позже даты приёмки",
		}
	}

	if !result.ExpiresAt.After(serverTime) {
		return result, &validations.FieldError{
			Field:   path + ".expires_at",
			Message: "Нельзя принять уже просроченную партию",
		}
	}

	return result, nil
}

func parseBatchDate(field, raw string) (time.Time, error) {
	value, err := validations.ParseDateTime(field, raw)
	if err != nil {
		return time.Time{}, err
	}

	if value.Year() < 1 || value.Year() > 9999 {
		return time.Time{}, &validations.FieldError{
			Field:   field,
			Message: "Дата должна находиться в диапазоне годов 0001–9999",
		}
	}

	return value.Truncate(time.Microsecond), nil
}
