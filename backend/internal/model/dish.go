package model

import "github.com/google/uuid"

type RecipeIngredient struct {
	ProductID          uuid.UUID
	ProductName        string
	Unit               string
	QuantityPerPortion string
	AvailableQuantity  string
	ExpiredQuantity    string
}

type Dish struct {
	ID            uuid.UUID
	Name          string
	Description   *string
	IsActive      bool
	Revision      int64
	RecipeVersion *int
	Recipe        []RecipeIngredient
}

type DishFilter struct {
	ID           *uuid.UUID
	Search       *string
	IsActive     *bool
	Availability string
	Page         int
	PageSize     int
}
