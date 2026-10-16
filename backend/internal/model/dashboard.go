package model

import "github.com/google/uuid"

type DashboardSettings struct {
	RestaurantName         string
	Timezone               string
	ExpiringThresholdHours int
}

type DashboardData struct {
	Preferences          map[uuid.UUID]HotOfferPreference
	Settings             DashboardSettings
	Stock                []ProcurementStock
	ExpiringBatchesCount int64
	ExpiredBatchesCount  int64
}
