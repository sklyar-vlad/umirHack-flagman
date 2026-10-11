package model

import (
	"time"

	"github.com/google/uuid"
)

type Actor struct {
	ID   uuid.UUID
	Role string
}

type Session struct {
	Actor     Actor
	CSRFToken string
	User      User
	ExpiresAt time.Time
	Settings  DashboardSettings
}

type User struct {
	ID          uuid.UUID
	Name        string
	Email       string
	Role        string
	IsActive    bool
	Revision    int
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

type Credentials struct {
	User         User
	PasswordHash string
}
