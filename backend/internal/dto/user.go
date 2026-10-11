package dto

type Role struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type RoleListResponse struct {
	Items []Role `json:"items"`
}

type UserListResponse struct {
	Items    []User         `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Meta     map[string]any `json:"meta"`
}

type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
	IsActive *bool  `json:"is_active,omitempty"`
}

type UpdateUserRequest struct {
	Name             *string `json:"name,omitempty"`
	Email            *string `json:"email,omitempty"`
	Role             *string `json:"role,omitempty"`
	IsActive         *bool   `json:"is_active,omitempty"`
	ExpectedRevision int     `json:"expected_revision"`
}

type UserPasswordRequest struct {
	Password         string `json:"password"`
	ExpectedRevision int    `json:"expected_revision"`
}
