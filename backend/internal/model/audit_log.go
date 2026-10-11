package model

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type AuditLogFilter struct {
	Page, PageSize   int
	Search           string
	ActorID          *uuid.UUID
	SystemActor      bool
	Action           AuditAction
	EntityType       AuditEntity
	EntityID         *uuid.UUID
	DateFrom, DateTo *time.Time
}
type AuditLogRecord struct {
	ID            uuid.UUID
	CreatedAt     time.Time
	ActorID       *uuid.UUID
	ActorName     *string
	Action        AuditAction
	EntityType    AuditEntity
	EntityID      *uuid.UUID
	RequestID     *string
	Before, After json.RawMessage
}
