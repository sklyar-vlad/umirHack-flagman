package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

const maxEventRequestIDLength = 128

var ErrTransactionRequired = errors.New("an explicit transaction is required for audit and outbox writes")

// AppendAudit writes through the caller's transaction. Its error must be returned
// from WithinTx so that the business mutation is rolled back together with audit.
func (r *repository) AppendAudit(ctx context.Context, q DBTX, entry *model.AuditEntry) error {
	tx, err := eventTransaction(q)
	if err != nil {
		return err
	}
	if err := validateAuditEntry(entry); err != nil {
		return err
	}
	requestID, err := eventRequestID(ctx, entry.ActorID)
	if err != nil {
		return err
	}
	before, err := marshalAuditSnapshot(entry.EntityType, entry.Before)
	if err != nil {
		return fmt.Errorf("prepare audit before snapshot: %w", err)
	}
	after, err := marshalAuditSnapshot(entry.EntityType, entry.After)
	if err != nil {
		return fmt.Errorf("prepare audit after snapshot: %w", err)
	}

	const query = `
		INSERT INTO audit_log (
			actor_id, action, entity_type, entity_id, request_id,
			before_data, after_data, created_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, clock_timestamp())
	`
	_, err = tx.Exec(ctx, query,
		eventUUID(entry.ActorID), string(entry.Action), string(entry.EntityType),
		eventUUID(entry.EntityID), requestID, before, after,
	)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// AppendOutbox saves an unpublished event in the business transaction. Publication
// time and SSE cursor are assigned separately by the future dispatcher after commit.
func (r *repository) AppendOutbox(ctx context.Context, q DBTX, event *model.OutboxEvent) error {
	tx, err := eventTransaction(q)
	if err != nil {
		return err
	}
	if event == nil {
		return errors.New("outbox event is required")
	}
	requestID, err := eventRequestID(ctx, event.ActorID)
	if err != nil {
		return err
	}
	if err := validateOutboxIDs(event); err != nil {
		return err
	}
	topics, err := outboxTopics(event)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(&model.OutboxPayload{
		OccurredAt:  time.Now().UTC(),
		RequestID:   requestID,
		Topics:      topics,
		ProductIDs:  event.ProductIDs,
		BatchIDs:    event.BatchIDs,
		DishIDs:     event.DishIDs,
		CategoryIDs: event.CategoryIDs,
		SupplyIDs:   event.SupplyIDs,
		OrderIDs:    event.OrderIDs,
		UserIDs:     event.UserIDs,
	})
	if err != nil {
		return fmt.Errorf("prepare outbox payload: %w", err)
	}

	const query = `
		INSERT INTO outbox_events (event_type, actor_id, payload, created_at)
		VALUES ($1, $2, $3, clock_timestamp())
	`
	_, err = tx.Exec(ctx, query, string(event.Type), eventUUID(event.ActorID), payload)
	if err != nil {
		return fmt.Errorf("append outbox: %w", err)
	}
	return nil
}

func eventTransaction(q DBTX) (pgx.Tx, error) {
	tx, ok := q.(pgx.Tx)
	if !ok || tx == nil {
		return nil, ErrTransactionRequired
	}
	// An interface containing a nil *pgxpool.Tx is not itself nil.
	value := reflect.ValueOf(tx)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, ErrTransactionRequired
	}
	return tx, nil
}

func eventRequestID(ctx context.Context, actorID *uuid.UUID) (string, error) {
	if actorID != nil && *actorID == uuid.Nil {
		return "", errors.New("actor ID must be a nonzero UUID or nil for a system operation")
	}
	requestID := requestctx.ID(ctx)
	if utf8.RuneCountInString(requestID) > maxEventRequestIDLength ||
		!utf8.ValidString(requestID) || strings.ContainsRune(requestID, 0) {
		return "", errors.New("invalid event request ID")
	}
	return requestID, nil
}

func eventUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func validateAuditEntry(entry *model.AuditEntry) error {
	if entry == nil {
		return errors.New("audit entry is required")
	}
	if !allowedAuditAction(entry.Action, entry.EntityType) {
		return errors.New("unsupported audit action for entity type")
	}
	if entry.EntityType == model.AuditSettings {
		if entry.EntityID != nil {
			return errors.New("settings audit entity ID must be nil")
		}
	} else if entry.EntityID == nil || *entry.EntityID == uuid.Nil {
		return errors.New("audit entity ID must be a nonzero UUID")
	}
	if entry.After == nil {
		return errors.New("audit after snapshot is required")
	}
	if entry.Action == model.AuditCreate && entry.Before != nil {
		return errors.New("create audit must not have a before snapshot")
	}
	if (entry.Action == model.AuditUpdate || entry.Action == model.AuditPasswordChange) && entry.Before == nil {
		return errors.New("update audit requires a before snapshot")
	}
	return nil
}

func allowedAuditAction(action model.AuditAction, entity model.AuditEntity) bool {
	switch action {
	case model.AuditCreate:
		switch entity {
		case model.AuditProductCategory, model.AuditProduct, model.AuditDish,
			model.AuditSupply, model.AuditOrder, model.AuditUser:
			return true
		}
	case model.AuditUpdate:
		switch entity {
		case model.AuditProductCategory, model.AuditProduct, model.AuditDish,
			model.AuditUser, model.AuditSettings:
			return true
		}
	case model.AuditPasswordChange:
		return entity == model.AuditUser
	case model.AuditManualWriteOff, model.AuditExpiration:
		return entity == model.AuditStockMovement
	}
	return false
}

func marshalAuditSnapshot(entity model.AuditEntity, snapshot model.AuditSnapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, nil
	}
	if !auditSnapshotMatches(entity, snapshot) {
		return nil, errors.New("audit snapshot must have the exact allowed type for its entity")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode audit snapshot: %w", err)
	}
	return data, nil
}

func auditSnapshotMatches(entity model.AuditEntity, snapshot model.AuditSnapshot) bool {
	// Checking exact types also rejects wrappers with extra fields or custom JSON
	// marshalers, even when they embed one of the allowed snapshot types.
	switch value := snapshot.(type) {
	case *model.CategoryAuditSnapshot:
		return value != nil && entity == model.AuditProductCategory
	case *model.ProductAuditSnapshot:
		return value != nil && entity == model.AuditProduct
	case *model.DishAuditSnapshot:
		return value != nil && entity == model.AuditDish
	case *model.SupplyAuditSnapshot:
		return value != nil && entity == model.AuditSupply
	case *model.OrderAuditSnapshot:
		return value != nil && entity == model.AuditOrder
	case *model.StockMovementAuditSnapshot:
		return value != nil && entity == model.AuditStockMovement
	case *model.UserAuditSnapshot:
		return value != nil && entity == model.AuditUser
	case *model.SettingsAuditSnapshot:
		return value != nil && entity == model.AuditSettings
	}
	return false
}

func validateOutboxIDs(event *model.OutboxEvent) error {
	groups := [][]uuid.UUID{
		event.ProductIDs, event.BatchIDs, event.DishIDs, event.CategoryIDs,
		event.SupplyIDs, event.OrderIDs, event.UserIDs,
	}
	for _, ids := range groups {
		for _, id := range ids {
			if id == uuid.Nil {
				return errors.New("outbox identifiers must be nonzero UUIDs")
			}
		}
	}
	if event.Type == model.UsersChanged {
		if len(event.UserIDs) == 0 || len(event.ProductIDs)+len(event.BatchIDs)+len(event.DishIDs)+
			len(event.CategoryIDs)+len(event.SupplyIDs)+len(event.OrderIDs) != 0 {
			return errors.New("users event requires user IDs and permits no other identifiers")
		}
	} else if len(event.UserIDs) != 0 {
		return errors.New("user IDs are permitted only in a users event")
	}
	return nil
}

func inventoryTopics() []model.EventTopic {
	return []model.EventTopic{
		model.TopicInventory, model.TopicBatches, model.TopicDashboard, model.TopicDishes,
		model.TopicStopList, model.TopicHotOffers, model.TopicProcurementRecommendations,
	}
}

func outboxTopics(event *model.OutboxEvent) ([]model.EventTopic, error) {
	switch event.Type {
	case model.InventoryChanged:
		topics := append(inventoryTopics(), model.TopicStockMovements)
		if len(event.SupplyIDs) != 0 {
			topics = append(topics, model.TopicSupplies)
		}
		if len(event.OrderIDs) != 0 {
			topics = append(topics, model.TopicOrders)
		}
		return topics, nil
	case model.InventoryTimeChanged:
		return inventoryTopics(), nil
	case model.ProductsChanged:
		return append(inventoryTopics(), model.TopicProducts), nil
	case model.CategoriesChanged:
		return []model.EventTopic{model.TopicProductCategories, model.TopicProducts}, nil
	case model.DishesChanged:
		return []model.EventTopic{
			model.TopicDishes, model.TopicStopList, model.TopicHotOffers,
			model.TopicDashboard, model.TopicProcurementRecommendations,
		}, nil
	case model.SettingsChanged:
		return append(inventoryTopics(), model.TopicSettings), nil
	case model.UsersChanged:
		return []model.EventTopic{model.TopicUsers}, nil
	}
	return nil, errors.New("unsupported outbox event type")
}
