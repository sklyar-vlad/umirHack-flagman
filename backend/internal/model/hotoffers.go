package model

type HotOffersFilter struct {
	Search   *string
	Page     int
	PageSize int
}

type HotOfferPreference struct {
	Published bool   `json:"is_published"`
	Manual    bool   `json:"is_manual"`
	Message   string `json:"message"`
	Revision  int    `json:"revision"`
}
