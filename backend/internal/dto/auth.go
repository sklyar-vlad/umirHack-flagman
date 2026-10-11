package dto

import (
	"time"

	"github.com/google/uuid"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type User struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	Revision    int        `json:"revision"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

type AuthResponse struct {
	User             User               `json:"user"`
	CSRFToken        string             `json:"csrf_token"`
	SessionExpiresAt time.Time          `json:"session_expires_at"`
	Settings         *DashboardSettings `json:"settings,omitempty"`
}
