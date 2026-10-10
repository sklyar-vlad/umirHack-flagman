package validations

const (
	MaxEntries  = 100
	MaxPortions = 1000
)

func ValidateCount(field string, count int) error {
	if count < 1 || count > MaxEntries {
		return &FieldError{
			Field:   field,
			Message: "Допустимо от 1 до 100 элементов",
		}
	}

	return nil
}

func ValidatePortions(field string, portions int) error {
	if portions < 1 || portions > MaxPortions {
		return &FieldError{
			Field:   field,
			Message: "Допустимо от 1 до 1000 порций одного блюда",
		}
	}

	return nil
}
