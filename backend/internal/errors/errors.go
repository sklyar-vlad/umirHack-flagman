package errors

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrConflict            = errors.New("conflict")
	ErrUnavailable         = errors.New("unavailable")
	ErrShelfLifeRequired   = errors.New("shelf life is required")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrRequestInProgress   = errors.New("request in progress")
	ErrInvalidJSON         = errors.New("invalid JSON")
	ErrBodyTooLarge        = errors.New("request body too large")
	ErrTooManyRequests     = errors.New("too many requests")
)
