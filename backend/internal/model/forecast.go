package model

import "github.com/google/uuid"

type ForecastFilter struct {
	LookbackDays, HorizonHours int
	Page, PageSize             int
}

// ConsumptionTotal preserves the recorded unit; incompatible history cannot
// provide a rate for the current ingredient.
type ConsumptionTotal struct {
	ProductID      uuid.UUID
	Unit, Quantity string
}
