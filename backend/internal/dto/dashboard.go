package dto

type DashboardSettings struct {
	RestaurantName         string `json:"restaurant_name"`
	Timezone               string `json:"timezone"`
	ExpiringThresholdHours int    `json:"expiring_threshold_hours"`
}

type DashboardResponse struct {
	ExpiringBatchesCount       int64             `json:"expiring_batches_count"`
	ExpiredBatchesCount        int64             `json:"expired_batches_count"`
	StopListDishesCount        int64             `json:"stop_list_dishes_count"`
	OutOfStockProductsCount    int64             `json:"out_of_stock_products_count"`
	AttentionBatches           []AttentionBatch  `json:"attention_batches"`
	AttentionTotal             int64             `json:"attention_total"`
	HotOffers                  []HotOffer        `json:"hot_offers"`
	ProcurementRecommendations []ProcurementItem `json:"procurement_recommendations"`
	Settings                   DashboardSettings `json:"settings"`
	Meta                       map[string]any    `json:"meta"`
}
