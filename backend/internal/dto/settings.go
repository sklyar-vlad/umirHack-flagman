package dto

type Settings struct {
	RestaurantName         string `json:"restaurant_name"`
	Timezone               string `json:"timezone"`
	ExpiringThresholdHours int    `json:"expiring_threshold_hours"`
	AutoWriteOffExpired    bool   `json:"auto_writeoff_expired"`
	Revision               int    `json:"revision"`
}

type UpdateSettingsRequest struct {
	RestaurantName         *string `json:"restaurant_name"`
	Timezone               *string `json:"timezone"`
	ExpiringThresholdHours *int    `json:"expiring_threshold_hours"`
	AutoWriteOffExpired    *bool   `json:"auto_writeoff_expired"`
	ExpectedRevision       int     `json:"expected_revision"`
}
