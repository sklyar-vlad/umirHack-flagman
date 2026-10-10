package model

import (
	"time"

	"github.com/google/uuid"
)

type ProductCategory struct {
	ID   uuid.UUID
	Name string
}

type Product struct {
	ID                     uuid.UUID
	Name                   string
	Category               ProductCategory
	Unit                   string
	DisplayUnit            string
	StandardShelfLifeHours *int
	TargetStock            *string
	IsActive               bool
	Revision               int64
	CreatedAt              time.Time
}

type ProductFilter struct {
	Search     *string
	CategoryID *uuid.UUID
	IsActive   *bool
	Page       int
	PageSize   int
}
