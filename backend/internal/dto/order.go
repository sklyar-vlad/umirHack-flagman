package dto

import (
	"time"

	"github.com/google/uuid"
)

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

type OrderAllocation struct {
	MovementID                  uuid.UUID `json:"movement_id"`
	ProductID                   uuid.UUID `json:"product_id"`
	ProductName                 string    `json:"product_name"`
	Unit                        string    `json:"unit"`
	BatchID                     uuid.UUID `json:"batch_id"`
	SupplyID                    uuid.UUID `json:"supply_id"`
	SupplyNumber                string    `json:"supply_number"`
	LineNumber                  int       `json:"line_number"`
	Quantity                    string    `json:"quantity"`
	BatchRemainingQuantityAfter string    `json:"batch_remaining_quantity_after"`
}

type CreatedOrderItem struct {
	OrderItemSnapshot
	ID          uuid.UUID         `json:"id"`
	Allocations []OrderAllocation `json:"allocations"`
}

type OrderDetail struct {
	ID        uuid.UUID          `json:"id"`
	Number    string             `json:"number"`
	Status    string             `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	CreatedBy ActorBrief         `json:"created_by"`
	Items     []CreatedOrderItem `json:"items"`
}
