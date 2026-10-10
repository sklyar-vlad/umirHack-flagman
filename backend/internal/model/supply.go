package model

import (
	"time"

	"github.com/google/uuid"
)

type SupplyStatus string

const (
	SupplyStatusReceived SupplyStatus = "received"
)

type BatchStatus string

const (
	BatchStatusValid    BatchStatus = "valid"
	BatchStatusExpiring BatchStatus = "expiring"
	BatchStatusExpired  BatchStatus = "expired"
	BatchStatusDepleted BatchStatus = "depleted"
)

type ExpirySource string

const (
	ExpirySourceExplicit   ExpirySource = "explicit"
	ExpirySourceCalculated ExpirySource = "calculated"
)

type ActorBrief struct {
	ID   uuid.UUID `json:"id" db:"id"`
	Name string    `json:"name" db:"name"`
}

type Supply struct {
	ID         uuid.UUID    `json:"id" db:"id"`
	Number     string       `json:"number" db:"number"`
	CreatedAt  time.Time    `json:"created_at" db:"created_at"`
	ReceivedAt time.Time    `json:"received_at" db:"received_at"`
	CreatedBy  ActorBrief   `json:"created_by"`
	CreatedByID uuid.UUID   `json:"-" db:"created_by"`
	Status     SupplyStatus `json:"status" db:"status"`
	ItemsCount int          `json:"items_count" db:"items_count"`
}

type BatchItem struct {
	BatchID           uuid.UUID    `db:"batch_id"`
	ProductID         uuid.UUID    `db:"product_id"`
	ProductName       string       `db:"product_name"`
	Unit              string       `db:"unit"`
	DisplayUnit       string       `db:"display_unit"`
	LineNumber        int          `db:"line_number"`
	InitialQuantity   string       `db:"initial_quantity"`
	RemainingQuantity string       `db:"remaining_quantity"`
	ProducedAt        *time.Time   `db:"produced_at"`
	ShelfLifeHours    *int         `db:"shelf_life_hours"`
	ExpirySource      ExpirySource `db:"expiry_source"`
	ExpiresAt         time.Time    `db:"expires_at"`
	CreatedAt         time.Time    `db:"created_at"`
}

type SupplyFilter struct {
	Search   *string
	DateFrom *time.Time
	DateTo   *time.Time
	Page     int
	PageSize int
}