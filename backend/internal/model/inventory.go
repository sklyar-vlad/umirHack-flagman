package model

import (
	"time"

	"github.com/google/uuid"
)

type InventoryFilter struct {
	Search     *string
	CategoryID *uuid.UUID
	Filter     string
	Sort       string
	Page       int
	PageSize   int
}

type InventoryItem struct {
	Product           Product
	PhysicalQuantity  string
	AvailableQuantity string
	ExpiringQuantity  string
	ExpiredQuantity   string
	TargetStock       *string
	DeficitQuantity   *string
	BatchesCount      int64
	NearestExpiresAt  *time.Time
}

type InventoryCounts struct {
	All        int64
	Expiring   int64
	Expired    int64
	OutOfStock int64
	Deficit    int64
}

type InventoryAttentionBatch struct {
	BatchID           uuid.UUID
	ProductID         uuid.UUID
	ProductName       string
	SupplyID          uuid.UUID
	SupplyNumber      string
	LineNumber        int
	RemainingQuantity string
	Unit              string
	ExpiresAt         time.Time
}

type InventoryResult struct {
	Items            []InventoryItem
	Total            int64
	Counts           InventoryCounts
	AttentionBatches []InventoryAttentionBatch
	AttentionTotal   int64
}
