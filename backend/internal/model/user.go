package model

// UserFilter selects safe user fields; password hashes never belong to this view.
type UserFilter struct {
	Page, PageSize int
	Search         *string
	Role           *string
	IsActive       *bool
	Descending     bool
}
