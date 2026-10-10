package validations

import (
	"math"
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

	pagination := dto.Pagination{
		Page:     page,
		PageSize: pageSize,
	}
	if _, err := PaginationOffset(pagination); err != nil {
		return dto.Pagination{}, err
	}
	return pagination, nil
}

// PaginationOffset guards the BIGINT offset before multiplying page and size.
func PaginationOffset(pagination dto.Pagination) (int64, error) {
	if pagination.Page < 1 {
		return 0, &FieldError{Field: "page", Message: "Ожидается целое число больше нуля"}
	}
	if pagination.PageSize < 1 || pagination.PageSize > 100 {
		return 0, &FieldError{Field: "page_size", Message: "Допустимое значение: от 1 до 100"}
	}
	page := int64(pagination.Page - 1)
	size := int64(pagination.PageSize)
	if page > math.MaxInt64/size {
		return 0, &FieldError{Field: "page", Message: "Номер страницы слишком велик"}
	}
	return page * size, nil
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
