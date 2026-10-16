package dto

import (
	"time"

	"github.com/google/uuid"
)

type WriteOffRequest struct {
	BatchID  string `json:"batch_id"`
	Quantity string `json:"quantity"`
	Reason   string `json:"reason"`
}

type StockMovement struct {
	ID                     uuid.UUID   `json:"id"`
	BatchID                uuid.UUID   `json:"batch_id"`
	ProductID              uuid.UUID   `json:"product_id"`
	OrderID                *uuid.UUID  `json:"order_id"`
	OrderItemID            *uuid.UUID  `json:"order_item_id"`
	Type                   string      `json:"type"`
	DeltaQuantity          string      `json:"delta_quantity"`
	RemainingQuantityAfter string      `json:"remaining_quantity_after"`
	ProductNameSnapshot    string      `json:"product_name"`
	UnitSnapshot           string      `json:"unit"`
	Actor                  *ActorBrief `json:"actor"`
	OccurredAt             time.Time   `json:"occurred_at"`
	Note                   *string     `json:"note"`
}
