package validations

import "time"

func ParseDateTime(field, raw string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, &FieldError{
			Field:   field,
			Message: "Ожидается дата и время с timezone, например 2026-10-10T15:30:00+03:00",
		}
	}

	return value.UTC(), nil
}
