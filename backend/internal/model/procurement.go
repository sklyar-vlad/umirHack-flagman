package model

import "github.com/google/uuid"

type ProcurementFilter struct {
	Search     *string
	CategoryID *uuid.UUID
	Page       int
	PageSize   int
}

type ProcurementStock struct {
	Product           Product
	AvailableQuantity string
	ExpiredQuantity   string
}
