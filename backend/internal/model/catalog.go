package model

import "github.com/google/uuid"

type CatalogCategory struct {
	ID       uuid.UUID
	Name     string
	Revision int
}

type CategoryFilter struct {
	Search   *string
	Page     int
	PageSize int
}
