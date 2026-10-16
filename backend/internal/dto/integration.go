package dto

import (
	"time"

	"github.com/google/uuid"
)

type IntegrationToken struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	Token     string     `json:"token,omitempty"`
}
type ExternalProductRequest struct {
	ExternalID string               `json:"external_id"`
	Product    CreateProductRequest `json:"product"`
}
type IncomingSupplyRequest struct {
	ExternalID string               `json:"external_id"`
	Number     string               `json:"number"`
	Items      []IncomingSupplyLine `json:"items"`
}
type IncomingSupplyLine struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  string    `json:"quantity"`
	ExpiresAt string    `json:"expires_at"`
}
type IncomingSupplyItem struct {
	ID               uuid.UUID `json:"id"`
	ProductID        uuid.UUID `json:"product_id"`
	ProductName      string    `json:"product_name"`
	Unit             string    `json:"unit"`
	Quantity         string    `json:"quantity"`
	AcceptedQuantity string    `json:"accepted_quantity"`
	ExpiresAt        time.Time `json:"expires_at"`
	LineNumber       int       `json:"line_number"`
}
type IncomingSupply struct {
	ID         uuid.UUID            `json:"id"`
	ExternalID string               `json:"external_id"`
	Number     string               `json:"number"`
	Source     string               `json:"source"`
	Status     string               `json:"status"`
	Revision   int                  `json:"revision"`
	CreatedAt  time.Time            `json:"created_at"`
	Items      []IncomingSupplyItem `json:"items"`
	ReceiptIDs []uuid.UUID          `json:"receipt_ids"`
}
type AcceptIncomingSupply struct {
	ExpectedRevision int                        `json:"expected_revision"`
	Items            []AcceptIncomingSupplyItem `json:"items"`
}
type AcceptIncomingSupplyItem struct {
	ID       uuid.UUID `json:"id"`
	Quantity string    `json:"quantity"`
}
type IntegrationPrincipal struct {
	TokenID uuid.UUID
	ActorID uuid.UUID
}
