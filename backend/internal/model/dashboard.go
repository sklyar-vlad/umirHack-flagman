package model

type DashboardSettings struct {
	RestaurantName         string
	Timezone               string
	ExpiringThresholdHours int
}

type DashboardData struct {
	Settings             DashboardSettings
	Stock                []ProcurementStock
	ExpiringBatchesCount int64
	ExpiredBatchesCount  int64
}
