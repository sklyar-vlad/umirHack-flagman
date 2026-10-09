package validations

type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string {
	return e.Field + ": " + e.Message
}
