package dto

import (
	"time"

	"github.com/google/uuid"
)

type HotOfferDish struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	RecipeVersion int       `json:"recipe_version"`
}

type HotOfferBatch struct {
	BatchID           uuid.UUID `json:"batch_id"`
	ProductID         uuid.UUID `json:"product_id"`
	ProductName       string    `json:"product_name"`
	SupplyID          uuid.UUID `json:"supply_id"`
	SupplyNumber      string    `json:"supply_number"`
	LineNumber        int       `json:"line_number"`
	Unit              string    `json:"unit"`
	RemainingQuantity string    `json:"remaining_quantity"`
	UsedForOnePortion string    `json:"used_for_one_portion"`
	ExpiresAt         time.Time `json:"expires_at"`
	RemainingSeconds  int64     `json:"remaining_seconds"`
}

type HotOffer struct {
	Dish              HotOfferDish    `json:"dish"`
	Rank              int64           `json:"rank"`
	PriorityScore     string          `json:"priority_score"`
	PriorityLevel     string          `json:"priority_level"`
	AvailablePortions string          `json:"available_portions"`
	Reason            string          `json:"reason"`
	ProblemBatches    []HotOfferBatch `json:"problem_batches"`
}

type HotOffersListResponse = ListResponse[HotOffer]
