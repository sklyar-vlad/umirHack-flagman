package dto

import "github.com/google/uuid"

type InventoryProduct struct {
	ProductBrief
	Category CategoryBrief `json:"category"`
	IsActive bool          `json:"is_active"`
}

type InventoryItem struct {
	Product           InventoryProduct `json:"product"`
	PhysicalQuantity  string           `json:"physical_quantity"`
	AvailableQuantity string           `json:"available_quantity"`
	ExpiringQuantity  string           `json:"expiring_quantity"`
	ExpiredQuantity   string           `json:"expired_quantity"`
	TargetStock       *string          `json:"target_stock"`
	DeficitQuantity   *string          `json:"deficit_quantity"`
	BatchesCount      int64            `json:"batches_count"`
	NearestExpiresAt  *string          `json:"nearest_expires_at"`
	RemainingSeconds  *int64           `json:"remaining_seconds"`
	HasExpiring       bool             `json:"has_expiring"`
	HasExpired        bool             `json:"has_expired"`
	IsOutOfStock      bool             `json:"is_out_of_stock"`
	Status            string           `json:"status"`
}

type InventoryCounts struct {
	All        int64 `json:"all"`
	Expiring   int64 `json:"expiring"`
	Expired    int64 `json:"expired"`
	OutOfStock int64 `json:"out_of_stock"`
	Deficit    int64 `json:"deficit"`
}

type AttentionBatch struct {
	BatchID           uuid.UUID `json:"batch_id"`
	ProductID         uuid.UUID `json:"product_id"`
	ProductName       string    `json:"product_name"`
	SupplyID          uuid.UUID `json:"supply_id"`
	SupplyNumber      string    `json:"supply_number"`
	LineNumber        int       `json:"line_number"`
	RemainingQuantity string    `json:"remaining_quantity"`
	Unit              string    `json:"unit"`
	Status            string    `json:"status"`
	ExpiresAt         string    `json:"expires_at"`
	RemainingSeconds  int64     `json:"remaining_seconds"`
}

type InventoryListResponse struct {
	ListResponse[InventoryItem]
	Counts           InventoryCounts  `json:"counts"`
	AttentionBatches []AttentionBatch `json:"attention_batches"`
	AttentionTotal   int64            `json:"attention_total"`
}
