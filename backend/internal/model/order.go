package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OrderBatch struct {
	ID, ProductID, SupplyID uuid.UUID
	SupplyNumber            string
	LineNumber              int
	RemainingQuantity       string
	ExpiresAt               time.Time
	ReceivedAt              time.Time
}

type OrderRecord struct {
	ID        uuid.UUID
	Number    int64
	CreatedAt time.Time
	CreatedBy ActorBrief
}

type NewOrderItem struct {
	ID, OrderID, DishID, RecipeVersionID uuid.UUID
	DishName                             string
	Portions                             int
}

type OrderConsumption struct {
	ID, ItemID, BatchID, ActorID     uuid.UUID
	Quantity, RemainingQuantityAfter string
	ProductName, Unit                string
	OccurredAt                       time.Time
}

type OrderAttempt struct {
	ID, ActorID                      uuid.UUID
	CreatedAt                        time.Time
	ReasonCode                       string
	ItemsSnapshot, Shortages, Issues json.RawMessage
}
