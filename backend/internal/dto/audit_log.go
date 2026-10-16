package dto

import (
	"encoding/json"

	"github.com/google/uuid"
)

type AuditActor struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type AuditLogEntry struct {
	ID         uuid.UUID       `json:"id"`
	CreatedAt  string          `json:"created_at"`
	Actor      *AuditActor     `json:"actor"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   *uuid.UUID      `json:"entity_id"`
	RequestID  *string         `json:"request_id"`
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
}
