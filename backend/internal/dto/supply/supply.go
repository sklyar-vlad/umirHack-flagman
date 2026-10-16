package supply

import (
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

type ActorBriefDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ProductBriefDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	DisplayUnit string    `json:"display_unit"`
}

type SupplyBatchParentDTO struct {
	ID         uuid.UUID `json:"id"`
	Number     string    `json:"number"`
	ReceivedAt string    `json:"received_at"`
}

type BatchDetailDTO struct {
	ID                uuid.UUID            `json:"id"`
	ProductID         uuid.UUID            `json:"product_id"`
	Unit              string               `json:"unit"`
	Supply            SupplyBatchParentDTO `json:"supply"`
	LineNumber        int                  `json:"line_number"`
	InitialQuantity   string               `json:"initial_quantity"`
	RemainingQuantity string               `json:"remaining_quantity"`
	ProducedAt        *string              `json:"produced_at"`
	ShelfLifeHours    *int                 `json:"shelf_life_hours"`
	ExpirySource      string               `json:"expiry_source"`
	ExpiresAt         string               `json:"expires_at"`
	RemainingSeconds  int64                `json:"remaining_seconds"`
	Status            string               `json:"status"`
	CreatedAt         string               `json:"created_at"`
}

type SupplyItemDetailDTO struct {
	Product ProductBriefDTO `json:"product"`
	Batch   BatchDetailDTO  `json:"batch"`
}

type SupplySummaryDTO struct {
	ID         uuid.UUID     `json:"id"`
	Number     string        `json:"number"`
	CreatedAt  string        `json:"created_at"`
	ReceivedAt string        `json:"received_at"`
	ItemsCount int           `json:"items_count"`
	CreatedBy  ActorBriefDTO `json:"created_by"`
	Status     string        `json:"status"`
}

type SupplyDetailDTO struct {
	SupplySummaryDTO
	Items []SupplyItemDetailDTO `json:"items"`
	Meta  SupplyDetailMetaDTO   `json:"meta"`
}

type SupplyDetailMetaDTO struct {
	ServerTime string `json:"server_time"`
}

type SupplyListResponse = dto.ListResponse[SupplySummaryDTO]
