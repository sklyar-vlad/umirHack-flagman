package validations

import (
	"net/url"
	"slices"
)

func ParseEnum(query url.Values, field string, defaultValue string, allowed ...string) (string, error) {
	value, err := readQueryValue(query, field, defaultValue)
	if err != nil {
		return "", err
	}

	if !slices.Contains(allowed, value) {
		return "", &FieldError{
			Field:   field,
			Message: "Недопустимое значение фильтра",
		}
	}

	return value, nil
}
