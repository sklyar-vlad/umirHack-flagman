package model

import (
	"time"

	"github.com/google/uuid"
)

type BatchFilter struct {
	ProductID uuid.UUID
	SupplyID  *uuid.UUID
	Status    string
	Sort      string
	Page      int
	PageSize  int
}

type Batch struct {
	ID                uuid.UUID
	ProductID         uuid.UUID
	Unit              string
	SupplyID          uuid.UUID
	SupplyNumber      string
	ReceivedAt        time.Time
	LineNumber        int
	InitialQuantity   string
	RemainingQuantity string
	Expiry            BatchExpiry
	CreatedAt         time.Time
}
