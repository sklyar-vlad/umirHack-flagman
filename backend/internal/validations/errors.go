package validations

const singleValueMessage = "Параметр должен быть указан один раз"

type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string {
	return e.Field + ": " + e.Message
}
