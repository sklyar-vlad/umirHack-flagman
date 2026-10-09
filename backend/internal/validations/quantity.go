package validations

import (
	"regexp"
	"strconv"
	"strings"
)

var quantityPattern = regexp.MustCompile(
	`^\d{1,11}(\.\d{1,3})?$`,
)

func ParseQuantity(field, raw string) (int64, error) {
	if !quantityPattern.MatchString(raw) {
		return 0, &FieldError{
			Field: field,
			Message: "Ожидается неотрицательное десятичное число: " +
				"до 11 цифр до точки и до 3 после",
		}
	}

	whole, fraction, _ := strings.Cut(raw, ".")

	fraction += strings.Repeat("0", 3-len(fraction))

	value, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, &FieldError{
			Field:   field,
			Message: "Количество превышает допустимый диапазон",
		}
	}

	return value, nil
}
