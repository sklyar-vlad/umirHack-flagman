package order

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/fefo"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const orderStatusCompleted = "completed"

type CreateRepository interface {
	StockRepository
	InsertOrder(context.Context, repository.DBTX, uuid.UUID, uuid.UUID, time.Time) (model.OrderRecord, error)
	InsertItem(context.Context, repository.DBTX, *model.NewOrderItem) error
	Consume(context.Context, repository.DBTX, *model.OrderConsumption) error
	InsertAttempt(context.Context, repository.DBTX, *model.OrderAttempt) error
}

type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}

type CreateService struct {
	repo        CreateRepository
	events      EventWriter
	idempotency *idempotency.Service
	now         func() time.Time
}

func NewCreateService(repo CreateRepository, events EventWriter, idem *idempotency.Service) *CreateService {
	return &CreateService{repo: repo, events: events, idempotency: idem, now: time.Now}
}

func (s *CreateService) CreateIdempotent(
	ctx context.Context,
	actor, key uuid.UUID,
	body []byte,
) (model.IdempotencyResponse, error) {
	if actor == uuid.Nil {
		return model.IdempotencyResponse{}, apperrors.ErrUnauthorized
	}
	request, err := decodeOrderBody(body)
	if err != nil {
		return model.IdempotencyResponse{}, err
	}
	items, err := normalizeBasket(request)
	if err != nil {
		return model.IdempotencyResponse{}, err
	}

	slices.SortFunc(items, func(a, b basketItem) int { return slices.Compare(a.dishID[:], b.dishID[:]) })
	scope := model.IdempotencyScope{UserID: actor, Method: http.MethodPost, Path: "/api/v1/orders", Key: key}
	return s.idempotency.Execute(ctx, scope, body, func(tx repository.DBTX) (idempotency.Outcome, error) {
		stock, err := lockBasket(ctx, s.repo, tx, items, s.now)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		check, err := checkBasket(items, stock.dishes)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		if !check.CanCreate {
			return s.saveRefusal(ctx, tx, actor, &check, stock.serverTime)
		}
		return s.saveOrder(ctx, tx, actor, items, stock)
	})
}

func decodeOrderBody(body []byte) (dto.OrderRequest, error) {
	if !json.Valid(body) {
		return dto.OrderRequest{}, apperrors.ErrInvalidJSON
	}
	if bytes.TrimSpace(body)[0] != '{' {
		return dto.OrderRequest{}, &validations.FieldError{Field: "body", Message: "Ожидается объект корзины"}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request dto.OrderRequest
	if err := decoder.Decode(&request); err != nil {
		return dto.OrderRequest{}, &validations.FieldError{
			Field:   "body",
			Message: "Некорректная структура корзины: ожидаются items, dish_id, целые portions и recipe_version",
		}
	}
	return request, nil
}

func (s *CreateService) saveOrder(
	ctx context.Context,
	tx repository.DBTX,
	actor uuid.UUID,
	items []basketItem,
	stock lockedStock,
) (idempotency.Outcome, error) {
	response := dto.OrderDetail{ID: uuid.New(), Status: orderStatusCompleted, Items: []dto.CreatedOrderItem{}}
	demands := []fefo.Demand{}
	positions := make(map[uuid.UUID]int, len(items))
	ingredients := make(map[uuid.UUID]model.RecipeIngredient)
	for _, item := range items {
		dish := stock.dishes[item.dishID]
		if dish.RecipeVersionID == nil || dish.RecipeVersion == nil {
			return idempotency.Outcome{}, fmt.Errorf("order recipe identity missing")
		}
		id := uuid.New()
		positions[id] = len(response.Items)
		response.Items = append(response.Items, dto.CreatedOrderItem{
			ID: id,
			OrderItemSnapshot: dto.OrderItemSnapshot{
				DishID: dish.ID, DishName: dish.Name, Portions: item.portions,
				RecipeVersion: dish.RecipeVersion,
			},
			Allocations: []dto.OrderAllocation{},
		})
		for _, ingredient := range dish.Recipe {
			ingredients[ingredient.ProductID] = ingredient
			perPortion, err := quantity.ParseMilli(ingredient.QuantityPerPortion)
			if err != nil {
				return idempotency.Outcome{}, err
			}
			demand := perPortion.Mul(perPortion, big.NewInt(int64(item.portions)))
			demands = append(
				demands,
				fefo.Demand{ItemID: id, ProductID: ingredient.ProductID, Quantity: quantity.FormatMilli(demand)},
			)
		}
	}
	plan, err := fefo.Plan(demands, stock.batches, stock.serverTime)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	created, err := s.repo.InsertOrder(ctx, tx, response.ID, actor, stock.serverTime)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	response.Number = strconv.FormatInt(created.Number, 10)
	response.CreatedAt = created.CreatedAt.UTC()
	response.CreatedBy = dto.ActorBrief{ID: created.CreatedBy.ID, Name: created.CreatedBy.Name}
	snapshot := &model.OrderAuditSnapshot{Number: created.Number, Items: []model.OrderItemAuditSnapshot{}}
	for _, item := range response.Items {
		dish := stock.dishes[item.DishID]
		if err := s.repo.InsertItem(ctx, tx, &model.NewOrderItem{
			ID: item.ID, OrderID: response.ID, DishID: item.DishID,
			RecipeVersionID: *dish.RecipeVersionID, DishName: item.DishName, Portions: item.Portions,
		}); err != nil {
			return idempotency.Outcome{}, err
		}
		snapshot.Items = append(snapshot.Items, model.OrderItemAuditSnapshot{
			DishID: item.DishID, RecipeVersionID: *dish.RecipeVersionID,
			DishNameSnapshot: item.DishName, Portions: item.Portions,
		})
	}
	products, batches, dishes := []uuid.UUID{}, []uuid.UUID{}, []uuid.UUID{}
	for i := range plan {
		allocation := &plan[i]
		item := &response.Items[positions[allocation.ItemID]]
		ingredient := ingredients[allocation.Batch.ProductID]
		movement := model.OrderConsumption{
			ID: uuid.New(), ItemID: item.ID, BatchID: allocation.Batch.ID, ActorID: actor,
			Quantity: allocation.Quantity, RemainingQuantityAfter: allocation.RemainingQuantityAfter,
			ProductName: ingredient.ProductName, Unit: ingredient.Unit, OccurredAt: stock.serverTime,
		}
		if err := s.repo.Consume(ctx, tx, &movement); err != nil {
			return idempotency.Outcome{}, err
		}
		b := allocation.Batch
		item.Allocations = append(item.Allocations, dto.OrderAllocation{
			MovementID: movement.ID, ProductID: b.ProductID, ProductName: ingredient.ProductName,
			Unit: ingredient.Unit, BatchID: b.ID, SupplyID: b.SupplyID,
			SupplyNumber: b.SupplyNumber, LineNumber: b.LineNumber, Quantity: allocation.Quantity,
			BatchRemainingQuantityAfter: allocation.RemainingQuantityAfter,
		})
		products = append(products, b.ProductID)
		batches = append(batches, b.ID)
		dishes = append(dishes, item.DishID)
	}
	if err := s.events.AppendAudit(ctx, tx, &model.AuditEntry{
		ActorID: &actor, Action: model.AuditCreate, EntityType: model.AuditOrder,
		EntityID: &response.ID, After: snapshot,
	}); err != nil {
		return idempotency.Outcome{}, err
	}
	if err := s.events.AppendOutbox(ctx, tx, &model.OutboxEvent{
		Type: model.InventoryChanged, ActorID: &actor, ProductIDs: uniqueIDs(products),
		BatchIDs: uniqueIDs(batches), DishIDs: uniqueIDs(dishes), OrderIDs: []uuid.UUID{response.ID},
	}); err != nil {
		return idempotency.Outcome{}, err
	}
	body, err := json.Marshal(response)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	return idempotency.Outcome{Response: model.IdempotencyResponse{
		Status: http.StatusCreated, Body: body, Location: "/api/v1/orders/" + response.ID.String(),
	}}, nil
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return slices.Compact(ids)
}
