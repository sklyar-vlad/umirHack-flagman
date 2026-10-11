package model

import (
	"github.com/google/uuid"
	"time"
)

type AnalyticsFilter struct {
	Page, PageSize   int
	DateFrom, DateTo *time.Time
	ProductID        *uuid.UUID
	Reason           string
}
