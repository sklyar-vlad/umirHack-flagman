package dto

import (
	"time"

	"github.com/google/uuid"
)

type OrderSummary struct {
	ID            uuid.UUID           `json:"id"`
	Number        string              `json:"number"`
	Status        string              `json:"status"`
	CreatedAt     time.Time           `json:"created_at"`
	CreatedBy     ActorBrief          `json:"created_by"`
	Items         []OrderItemSnapshot `json:"items"`
	TotalPortions int                 `json:"total_portions"`
}

type OrderAttempt struct {
	ID         uuid.UUID           `json:"id"`
	CreatedAt  time.Time           `json:"created_at"`
	CreatedBy  ActorBrief          `json:"created_by"`
	ReasonCode string              `json:"reason_code"`
	Items      []OrderItemSnapshot `json:"items"`
	Shortages  []Shortage          `json:"shortages"`
	Issues     []OrderIssue        `json:"issues"`
}
