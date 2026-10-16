package model

import "github.com/google/uuid"

type ForecastFilter struct {
	LookbackDays, HorizonHours int
	Page, PageSize             int
}

type ConsumptionTotal struct {
	ProductID      uuid.UUID
	Unit, Quantity string
}
