package dto

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	n.Set = true
	return json.Unmarshal(data, &n.Value)
}

type CatalogCategory struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Revision int       `json:"revision"`
}

type CategoryListResponse = ListResponse[CatalogCategory]

type CreateCategoryRequest struct {
	Name string `json:"name"`
}

type UpdateCategoryRequest struct {
	Name             string `json:"name"`
	ExpectedRevision int    `json:"expected_revision"`
}

type CreateProductRequest struct {
	Name                   string    `json:"name"`
	CategoryID             uuid.UUID `json:"category_id"`
	Unit                   string    `json:"unit"`
	DisplayUnit            string    `json:"display_unit"`
	StandardShelfLifeHours *int      `json:"standard_shelf_life_hours"`
	TargetStock            *string   `json:"target_stock"`
	IsActive               *bool     `json:"is_active"`
}

type UpdateProductRequest struct {
	Name                   *string          `json:"name"`
	CategoryID             *uuid.UUID       `json:"category_id"`
	Unit                   *string          `json:"unit"`
	DisplayUnit            *string          `json:"display_unit"`
	StandardShelfLifeHours Nullable[int]    `json:"standard_shelf_life_hours"`
	TargetStock            Nullable[string] `json:"target_stock"`
	IsActive               *bool            `json:"is_active"`
	ExpectedRevision       int              `json:"expected_revision"`
}
