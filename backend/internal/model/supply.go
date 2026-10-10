package model

import (
	"time"

	"github.com/google/uuid"
)

type SupplyProduct struct {
	ID                     uuid.UUID
	Name                   string
	Unit                   string
	IsActive               bool
	StandardShelfLifeHours *int
}

type ExpirySource string

const (
	ExpiryExplicit   ExpirySource = "explicit"
	ExpiryCalculated ExpirySource = "calculated"
)

type BatchExpiry struct {
	ProducedAt     *time.Time
	ShelfLifeHours *int
	Source         ExpirySource
	ExpiresAt      time.Time
}

type NewSupplyBatch struct {
	ID            uuid.UUID
	SupplyID      uuid.UUID
	ProductID     uuid.UUID
	LineNumber    int
	QuantityMilli int64
	Expiry        BatchExpiry
}

type NewSupply struct {
	ID         uuid.UUID
	Number     string
	ReceivedAt time.Time
	CreatedBy  uuid.UUID
}

type CreatedSupply struct {
	NewSupply
	CreatedAt  time.Time
	ServerTime time.Time
	Batches    []NewSupplyBatch
}

type SupplyDetails struct {
	NewSupply
	CreatedAt              time.Time
	ActorName              string
	ExpiringThresholdHours int
	Items                  []SupplyItemDetails
}

type SupplyItemDetails struct {
	ID                uuid.UUID
	ProductID         uuid.UUID
	ProductName       string
	Unit              string
	DisplayUnit       string
	LineNumber        int
	InitialQuantity   string
	RemainingQuantity string
	Expiry            BatchExpiry
	CreatedAt         time.Time
}

type NewSupplyReceipt struct {
	BatchID       uuid.UUID
	QuantityMilli int64
	ProductName   string
	Unit          string
	ActorID       uuid.UUID
	OccurredAt    time.Time
}
