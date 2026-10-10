package validations

import (
	"net/url"
	"strconv"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func ParsePagination(query url.Values) (dto.Pagination, error) {
	page, err := parsePositiveInt(query, "page", 1)
	if err != nil {
		return dto.Pagination{}, err
	}

	pageSize, err := parsePositiveInt(query, "page_size", 50)
	if err != nil {
		return dto.Pagination{}, err
	}

	if pageSize > 100 {
		return dto.Pagination{}, &FieldError{
			Field:   "page_size",
			Message: "Допустимое значение: от 1 до 100",
		}
	}

	return dto.Pagination{
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func parsePositiveInt(
	query url.Values,
	field string,
	defaultValue int,
) (int, error) {
	values, exists := query[field]
	if !exists {
		return defaultValue, nil
	}

	if len(values) != 1 {
		return 0, &FieldError{
			Field:   field,
			Message: singleValueMessage,
		}
	}

	value, err := strconv.Atoi(values[0])
	if err != nil || value < 1 {
		return 0, &FieldError{
			Field:   field,
			Message: "Ожидается целое число больше нуля",
		}
	}

	return value, nil
}
