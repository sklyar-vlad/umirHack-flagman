package dto

type ListResponse[T any] struct {
	Items    []T            `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Meta     map[string]any `json:"meta"`
}

type ErrorResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}
