package validations

import (
	"net/url"
	"slices"
	"strings"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func ParseSort(query url.Values, defaultValue string, allowed ...string) (dto.Sort, error) {
	raw, err := readQueryValue(query, "sort", defaultValue)
	if err != nil {
		return dto.Sort{}, err
	}

	if !slices.Contains(allowed, raw) {
		return dto.Sort{}, &FieldError{
			Field:   "sort",
			Message: "Недопустимое значение сортировки",
		}
	}

	return dto.Sort{
		Field:      strings.TrimPrefix(raw, "-"),
		Descending: strings.HasPrefix(raw, "-"),
	}, nil
}
