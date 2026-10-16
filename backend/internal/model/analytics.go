package model

import (
	"time"

	"github.com/google/uuid"
)

type AnalyticsFilter struct {
	Page, PageSize   int
	DateFrom, DateTo *time.Time
	ProductID        *uuid.UUID
	Reason           string
}
