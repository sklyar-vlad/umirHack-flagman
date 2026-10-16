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

type SupplyStatus string

const SupplyStatusReceived SupplyStatus = "received"

type ActorBrief struct {
	ID   uuid.UUID `json:"id"   db:"id"`
	Name string    `json:"name" db:"name"`
}

type Supply struct {
	ID          uuid.UUID    `json:"id"          db:"id"`
	Number      string       `json:"number"      db:"number"`
	CreatedAt   time.Time    `json:"created_at"  db:"created_at"`
	ReceivedAt  time.Time    `json:"received_at" db:"received_at"`
	CreatedBy   ActorBrief   `json:"created_by"`
	CreatedByID uuid.UUID    `json:"-"           db:"created_by"`
	Status      SupplyStatus `json:"status"      db:"status"`
	ItemsCount  int          `json:"items_count" db:"items_count"`
}

type SupplyFilter struct {
	Search   *string
	DateFrom *time.Time
	DateTo   *time.Time
	Page     int
	PageSize int
}
