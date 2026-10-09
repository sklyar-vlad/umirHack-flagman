package validations

import (
	"net/url"
	"slices"
)

func ValidateQuery(query url.Values, allowed ...string) error {
	for field, values := range query {
		if !slices.Contains(allowed, field) {
			return &FieldError{
				Field:   field,
				Message: "Неизвестный параметр запроса",
			}
		}

		if len(values) != 1 {
			return &FieldError{
				Field:   field,
				Message: "Параметр должен быть указан один раз",
			}
		}
	}

	return nil
}

func readQueryValue(
	query url.Values,
	field string,
	defaultValue string,
) (string, error) {
	values, exists := query[field]
	if !exists {
		return defaultValue, nil
	}

	if len(values) != 1 {
		return "", &FieldError{
			Field:   field,
			Message: "Параметр должен быть указан один раз",
		}
	}

	return values[0], nil
}
