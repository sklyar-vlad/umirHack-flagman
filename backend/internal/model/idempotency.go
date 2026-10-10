package model

import (
	"encoding/json"

	"github.com/google/uuid"
)

type IdempotencyScope struct {
	UserID uuid.UUID
	Method string
	Path   string
	Key    uuid.UUID
}

// Endpoint fits the existing endpoint column while keeping methods separate.
func (s IdempotencyScope) Endpoint() string { return s.Method + " " + s.Path }

type IdempotencyResponse struct {
	Status   int
	Body     json.RawMessage
	Location string
}

type IdempotencyRecord struct {
	RequestHash string
	Response    IdempotencyResponse
}
