package dto

import "github.com/google/uuid"

type WriteOffAnalytics struct {
	ProductID                 uuid.UUID `json:"product_id"`
	ProductName               string    `json:"product_name"`
	Unit                      string    `json:"unit"`
	ExpiredWrittenOffQuantity string    `json:"expired_written_off_quantity"`
	ManualWrittenOffQuantity  string    `json:"manual_written_off_quantity"`
	TotalWrittenOffQuantity   string    `json:"total_written_off_quantity"`
	MovementsCount            int64     `json:"movements_count"`
}

type IngredientShortageAnalytics struct {
	ProductID             uuid.UUID `json:"product_id"`
	ProductName           string    `json:"product_name"`
	Unit                  string    `json:"unit"`
	RejectedAttemptsCount int64     `json:"rejected_attempts_count"`
	TotalMissingQuantity  string    `json:"total_missing_quantity"`
}
