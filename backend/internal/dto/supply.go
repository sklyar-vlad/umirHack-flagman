package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateSupplyRequest struct {
	Number     string             `json:"number"`
	ReceivedAt string             `json:"received_at"`
	Items      []CreateSupplyItem `json:"items"`
}

type CreateSupplyItem struct {
	ProductID      string  `json:"product_id"`
	Quantity       string  `json:"quantity"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
	ProducedAt     *string `json:"produced_at,omitempty"`
	ShelfLifeHours *int    `json:"shelf_life_hours,omitempty"`
}

type ActorBrief struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ProductBrief struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	DisplayUnit string    `json:"display_unit"`
}

type SupplyReference struct {
	ID         uuid.UUID `json:"id"`
	Number     string    `json:"number"`
	ReceivedAt time.Time `json:"received_at"`
}

type SupplyBatch struct {
	ID                uuid.UUID       `json:"id"`
	ProductID         uuid.UUID       `json:"product_id"`
	Unit              string          `json:"unit"`
	Supply            SupplyReference `json:"supply"`
	LineNumber        int             `json:"line_number"`
	InitialQuantity   string          `json:"initial_quantity"`
	RemainingQuantity string          `json:"remaining_quantity"`
	ProducedAt        *time.Time      `json:"produced_at"`
	ShelfLifeHours    *int            `json:"shelf_life_hours"`
	ExpirySource      string          `json:"expiry_source"`
	ExpiresAt         time.Time       `json:"expires_at"`
	RemainingSeconds  int64           `json:"remaining_seconds"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
}

type SupplyDetailItem struct {
	Product ProductBrief `json:"product"`
	Batch   SupplyBatch  `json:"batch"`
}

type SupplyMeta struct {
	ServerTime time.Time `json:"server_time"`
}

type SupplyDetail struct {
	ID         uuid.UUID          `json:"id"`
	Number     string             `json:"number"`
	CreatedAt  time.Time          `json:"created_at"`
	ReceivedAt time.Time          `json:"received_at"`
	ItemsCount int                `json:"items_count"`
	CreatedBy  ActorBrief         `json:"created_by"`
	Status     string             `json:"status"`
	Items      []SupplyDetailItem `json:"items"`
	Meta       SupplyMeta         `json:"meta"`
}
