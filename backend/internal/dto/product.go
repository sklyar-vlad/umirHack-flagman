package dto

import "github.com/google/uuid"

type CategoryBrief struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Product struct {
	ProductBrief
	Category               CategoryBrief `json:"category"`
	StandardShelfLifeHours *int          `json:"standard_shelf_life_hours"`
	TargetStock            *string       `json:"target_stock"`
	IsActive               bool          `json:"is_active"`
	Revision               int64         `json:"revision"`
	CreatedAt              string        `json:"created_at"`
}

type ProductListResponse = ListResponse[Product]
