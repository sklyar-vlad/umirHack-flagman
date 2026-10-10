package dto

import "github.com/google/uuid"

type OrderRequest struct {
	Items []OrderRequestItem `json:"items"`
}

type OrderRequestItem struct {
	DishID        string `json:"dish_id"`
	Portions      int    `json:"portions"`
	RecipeVersion *int   `json:"recipe_version,omitempty"`
}

type OrderItemSnapshot struct {
	DishID        uuid.UUID `json:"dish_id"`
	DishName      string    `json:"dish_name"`
	Portions      int       `json:"portions"`
	RecipeVersion *int      `json:"recipe_version"`
}

type Requirement struct {
	ProductID         uuid.UUID `json:"product_id"`
	ProductName       string    `json:"product_name"`
	Unit              string    `json:"unit"`
	RequiredQuantity  string    `json:"required_quantity"`
	AvailableQuantity string    `json:"available_quantity"`
	MissingQuantity   string    `json:"missing_quantity"`
}

type OrderIssue struct {
	DishID               uuid.UUID `json:"dish_id"`
	Code                 string    `json:"code"`
	Message              string    `json:"message"`
	CurrentRecipeVersion *int      `json:"current_recipe_version"`
}

type OrderCheck struct {
	CanCreate    bool                `json:"can_create"`
	Items        []OrderItemSnapshot `json:"items"`
	Requirements []Requirement       `json:"requirements"`
	Shortages    []Shortage          `json:"shortages"`
	Issues       []OrderIssue        `json:"issues"`
	Meta         map[string]any      `json:"meta"`
}
