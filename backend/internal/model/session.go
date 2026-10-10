package model

import "github.com/google/uuid"

type Actor struct {
	ID   uuid.UUID
	Role string
}

type Session struct {
	Actor     Actor
	CSRFToken string
}
