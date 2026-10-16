package dto

import "github.com/google/uuid"

type AffectedDish struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ProcurementItem struct {
	ProductID           uuid.UUID      `json:"product_id"`
	ProductName         string         `json:"product_name"`
	Unit                string         `json:"unit"`
	DisplayUnit         string         `json:"display_unit"`
	AvailableQuantity   string         `json:"available_quantity"`
	ExpiredQuantity     string         `json:"expired_quantity"`
	TargetStock         string         `json:"target_stock"`
	RecommendedQuantity string         `json:"recommended_quantity"`
	ReasonCodes         []string       `json:"reason_codes"`
	Reason              string         `json:"reason"`
	AffectedDishes      []AffectedDish `json:"affected_dishes"`
}

type ProcurementListResponse = ListResponse[ProcurementItem]
