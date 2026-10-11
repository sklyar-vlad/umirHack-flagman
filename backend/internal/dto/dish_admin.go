package dto

type RecipeInput struct {
	ProductID          string `json:"product_id"`
	QuantityPerPortion string `json:"quantity_per_portion"`
}

type CreateDishRequest struct {
	Name        string        `json:"name"`
	Description *string       `json:"description"`
	IsActive    *bool         `json:"is_active"`
	Recipe      []RecipeInput `json:"recipe"`
}

type UpdateDishRequest struct {
	Name             *string          `json:"name"`
	Description      Nullable[string] `json:"description"`
	IsActive         *bool            `json:"is_active"`
	Recipe           *[]RecipeInput   `json:"recipe"`
	ExpectedRevision int              `json:"expected_revision"`
}

type PreviewDishRequest struct {
	Recipe []RecipeInput `json:"recipe"`
}

type DishPreview struct {
	IsAvailable       bool           `json:"is_available"`
	AvailablePortions string         `json:"available_portions"`
	Requirements      []Requirement  `json:"requirements"`
	Shortages         []Shortage     `json:"shortages"`
	Meta              map[string]any `json:"meta"`
}
