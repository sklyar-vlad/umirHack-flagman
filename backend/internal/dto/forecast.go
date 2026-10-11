package dto

import (
	"github.com/google/uuid"
	"time"
)

type StopListForecast struct {
	DishID            uuid.UUID  `json:"dish_id"`
	DishName          string     `json:"dish_name"`
	EstimatedStopAt   *time.Time `json:"estimated_stop_at"`
	Cause             string     `json:"cause"`
	LimitingProductID *uuid.UUID `json:"limiting_product_id"`
	Confidence        string     `json:"confidence"`
	Assumptions       []string   `json:"assumptions"`
}
