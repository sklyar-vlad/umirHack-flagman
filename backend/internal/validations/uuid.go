package validations

import (
	"github.com/google/uuid"
)

func ParseUUID(field, raw string) (uuid.UUID, error) {
	if len(raw) != 36 {
		return uuid.Nil, &FieldError{
			Field:   field,
			Message: "Ожидается UUID в формате xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		}
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, &FieldError{
			Field:   field,
			Message: "Некорректный UUID",
		}
	}

	return id, nil
}
