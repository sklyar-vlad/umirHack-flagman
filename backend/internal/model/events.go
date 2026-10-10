package model

import (
	"time"

	"github.com/google/uuid"
)

type AuditAction string

const (
	AuditCreate         AuditAction = "create"
	AuditUpdate         AuditAction = "update"
	AuditPasswordChange AuditAction = "password_change"
	AuditManualWriteOff AuditAction = "manual_writeoff"
	AuditExpiration     AuditAction = "expiration"
)

type AuditEntity string

const (
	AuditProductCategory AuditEntity = "product_category"
	AuditProduct         AuditEntity = "product"
	AuditDish            AuditEntity = "dish"
	AuditSupply          AuditEntity = "supply"
	AuditOrder           AuditEntity = "order"
	AuditStockMovement   AuditEntity = "stock_movement"
	AuditUser            AuditEntity = "user"
	AuditSettings        AuditEntity = "settings"
)

// AuditSnapshot accepts only explicitly defined projections of business data.
// Persistence also checks the concrete type before marshaling a snapshot.
type AuditSnapshot interface {
	auditSnapshot()
}

// AuditEntry is constructed by the service from authenticated identity and saved data.
// Request ID and creation time are assigned by the repository.
type AuditEntry struct {
	ActorID    *uuid.UUID
	Action     AuditAction
	EntityType AuditEntity
	EntityID   *uuid.UUID
	Before     AuditSnapshot
	After      AuditSnapshot
}

type CategoryAuditSnapshot struct {
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

type ProductAuditSnapshot struct {
	Name                   string    `json:"name"`
	CategoryID             uuid.UUID `json:"category_id"`
	Unit                   string    `json:"unit"`
	DisplayUnit            string    `json:"display_unit"`
	StandardShelfLifeHours *int      `json:"standard_shelf_life_hours"`
	TargetStock            *string   `json:"target_stock"`
	IsActive               bool      `json:"is_active"`
	Revision               int       `json:"revision"`
}

type RecipeIngredientAuditSnapshot struct {
	ProductID          uuid.UUID `json:"product_id"`
	QuantityPerPortion string    `json:"quantity_per_portion"`
}

type DishAuditSnapshot struct {
	Name            string                          `json:"name"`
	Description     *string                         `json:"description"`
	IsActive        bool                            `json:"is_active"`
	Revision        int                             `json:"revision"`
	RecipeVersionID *uuid.UUID                      `json:"recipe_version_id"`
	RecipeVersion   *int                            `json:"recipe_version"`
	Recipe          []RecipeIngredientAuditSnapshot `json:"recipe"`
}

type BatchAuditSnapshot struct {
	BatchID           uuid.UUID  `json:"batch_id"`
	ProductID         uuid.UUID  `json:"product_id"`
	LineNumber        int        `json:"line_number"`
	InitialQuantity   string     `json:"initial_quantity"`
	RemainingQuantity string     `json:"remaining_quantity"`
	ProducedAt        *time.Time `json:"produced_at"`
	ShelfLifeHours    *int       `json:"shelf_life_hours"`
	ExpirySource      string     `json:"expiry_source"`
	ExpiresAt         time.Time  `json:"expires_at"`
}

type SupplyAuditSnapshot struct {
	Number     string               `json:"number"`
	ReceivedAt time.Time            `json:"received_at"`
	Batches    []BatchAuditSnapshot `json:"batches"`
}

type OrderItemAuditSnapshot struct {
	DishID           uuid.UUID `json:"dish_id"`
	RecipeVersionID  uuid.UUID `json:"recipe_version_id"`
	DishNameSnapshot string    `json:"dish_name_snapshot"`
	Portions         int       `json:"portions"`
}

type OrderAuditSnapshot struct {
	Number int64                    `json:"number"`
	Items  []OrderItemAuditSnapshot `json:"items"`
}

type StockMovementAuditSnapshot struct {
	BatchID                uuid.UUID  `json:"batch_id"`
	OrderItemID            *uuid.UUID `json:"order_item_id"`
	Type                   string     `json:"type"`
	DeltaQuantity          string     `json:"delta_quantity"`
	RemainingQuantityAfter string     `json:"remaining_quantity_after"`
	ProductNameSnapshot    string     `json:"product_name_snapshot"`
	UnitSnapshot           string     `json:"unit_snapshot"`
	Note                   *string    `json:"note"`
}

// UserAuditSnapshot deliberately contains no password, hash, or session data.
type UserAuditSnapshot struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
	Revision int    `json:"revision"`
}

type SettingsAuditSnapshot struct {
	RestaurantName         string `json:"restaurant_name"`
	Timezone               string `json:"timezone"`
	ExpiringThresholdHours int    `json:"expiring_threshold_hours"`
	AutoWriteOffExpired    bool   `json:"auto_writeoff_expired"`
	Revision               int    `json:"revision"`
}

func (*CategoryAuditSnapshot) auditSnapshot()      {}
func (*ProductAuditSnapshot) auditSnapshot()       {}
func (*DishAuditSnapshot) auditSnapshot()          {}
func (*SupplyAuditSnapshot) auditSnapshot()        {}
func (*OrderAuditSnapshot) auditSnapshot()         {}
func (*StockMovementAuditSnapshot) auditSnapshot() {}
func (*UserAuditSnapshot) auditSnapshot()          {}
func (*SettingsAuditSnapshot) auditSnapshot()      {}

type OutboxEventType string

const (
	InventoryChanged     OutboxEventType = "inventory.changed"
	InventoryTimeChanged OutboxEventType = "inventory.time_changed"
	ProductsChanged      OutboxEventType = "products.changed"
	CategoriesChanged    OutboxEventType = "product_categories.changed"
	DishesChanged        OutboxEventType = "dishes.changed"
	SettingsChanged      OutboxEventType = "settings.changed"
	UsersChanged         OutboxEventType = "users.changed"
)

type EventTopic string

const (
	TopicInventory                  EventTopic = "inventory"
	TopicBatches                    EventTopic = "batches"
	TopicDashboard                  EventTopic = "dashboard"
	TopicDishes                     EventTopic = "dishes"
	TopicStopList                   EventTopic = "stop-list"
	TopicHotOffers                  EventTopic = "hot-offers"
	TopicProcurementRecommendations EventTopic = "procurement-recommendations"
	TopicProducts                   EventTopic = "products"
	TopicProductCategories          EventTopic = "product-categories"
	TopicSettings                   EventTopic = "settings"
	TopicUsers                      EventTopic = "users"
	TopicSupplies                   EventTopic = "supplies"
	TopicOrders                     EventTopic = "orders"
	TopicStockMovements             EventTopic = "stock-movements"
)

// OutboxEvent permits identifiers only. Topics, request ID, and time are assigned
// by the repository; callers cannot provide arbitrary payloads or topic names.
type OutboxEvent struct {
	Type        OutboxEventType
	ActorID     *uuid.UUID
	ProductIDs  []uuid.UUID
	BatchIDs    []uuid.UUID
	DishIDs     []uuid.UUID
	CategoryIDs []uuid.UUID
	SupplyIDs   []uuid.UUID
	OrderIDs    []uuid.UUID
	UserIDs     []uuid.UUID
}

// OutboxPayload is internal storage data. The SSE dispatcher must project its
// topics and identifiers according to the recipient's role before delivery.
type OutboxPayload struct {
	OccurredAt  time.Time    `json:"occurred_at"`
	RequestID   string       `json:"request_id,omitempty"`
	Topics      []EventTopic `json:"topics"`
	ProductIDs  []uuid.UUID  `json:"product_ids,omitempty"`
	BatchIDs    []uuid.UUID  `json:"batch_ids,omitempty"`
	DishIDs     []uuid.UUID  `json:"dish_ids,omitempty"`
	CategoryIDs []uuid.UUID  `json:"category_ids,omitempty"`
	SupplyIDs   []uuid.UUID  `json:"supply_ids,omitempty"`
	OrderIDs    []uuid.UUID  `json:"order_ids,omitempty"`
	UserIDs     []uuid.UUID  `json:"user_ids,omitempty"`
}
