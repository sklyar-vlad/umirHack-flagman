package dto

import "github.com/google/uuid"

type RecipeIngredient struct {
	ProductID          uuid.UUID `json:"product_id"`
	ProductName        string    `json:"product_name"`
	Unit               string    `json:"unit"`
	QuantityPerPortion string    `json:"quantity_per_portion"`
}

type Shortage struct {
	ProductID         uuid.UUID `json:"product_id"`
	ProductName       string    `json:"product_name"`
	Unit              string    `json:"unit"`
	RequiredQuantity  string    `json:"required_quantity"`
	AvailableQuantity string    `json:"available_quantity"`
	MissingQuantity   string    `json:"missing_quantity"`
	ExpiredQuantity   string    `json:"expired_quantity"`
	ReasonCode        string    `json:"reason_code"`
}

type Availability struct {
	IsAvailable           bool       `json:"is_available"`
	AvailablePortions     string     `json:"available_portions"`
	Shortages             []Shortage `json:"shortages"`
	UnavailabilityReasons []string   `json:"unavailability_reasons"`
}

type Dish struct {
	ID            uuid.UUID          `json:"id"`
	Name          string             `json:"name"`
	Description   *string            `json:"description"`
	IsActive      bool               `json:"is_active"`
	Revision      int64              `json:"revision"`
	RecipeVersion *int               `json:"recipe_version"`
	Recipe        []RecipeIngredient `json:"recipe"`
	Availability
	Meta map[string]any `json:"meta"`
}

type DishListResponse = ListResponse[Dish]
