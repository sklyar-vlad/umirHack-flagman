package model

import (
	"time"

	"github.com/google/uuid"
)

type WriteOffBatch struct {
	ID, ProductID                        uuid.UUID
	ProductName, Unit, RemainingQuantity string
}

type StockMovementFilter struct {
	ProductID, BatchID, OrderID *uuid.UUID
	Type                        string
	DateFrom, DateTo            *time.Time
	Page, PageSize              int
}
