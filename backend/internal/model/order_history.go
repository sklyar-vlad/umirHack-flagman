package model

import (
	"time"

	"github.com/google/uuid"
)

type OrderHistoryFilter struct {
	Page, PageSize   int
	Search           string
	ReasonCode       string
	DateFrom, DateTo *time.Time
}

type OrderHistory struct {
	OrderRecord
	Items []OrderHistoryItem
}

type OrderHistoryItem struct {
	NewOrderItem
	RecipeVersion int
	Allocations   []OrderHistoryAllocation
}

type OrderHistoryAllocation struct {
	MovementID, ProductID, BatchID, SupplyID uuid.UUID
	ProductName, Unit, SupplyNumber          string
	LineNumber                               int
	Quantity, RemainingQuantityAfter         string
}

type OrderAttemptHistory struct {
	OrderAttempt
	ActorName string
}
