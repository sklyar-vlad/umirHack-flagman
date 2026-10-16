package model

type UserFilter struct {
	Page, PageSize int
	Search         *string
	Role           *string
	IsActive       *bool
	Descending     bool
}
