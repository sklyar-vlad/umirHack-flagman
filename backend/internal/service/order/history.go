package order

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type HistoryRepository interface {
	ListOrders(context.Context, repository.DBTX, model.OrderHistoryFilter) ([]model.OrderHistory, int64, error)
	FindOrder(context.Context, repository.DBTX, uuid.UUID) (model.OrderHistory, error)
	ListAttempts(context.Context, repository.DBTX, model.OrderHistoryFilter) ([]model.OrderAttemptHistory, int64, error)
}

type HistoryService struct {
	repo         HistoryRepository
	transactions Transactions
}

func NewHistoryService(repo HistoryRepository, transactions Transactions) *HistoryService {
	return &HistoryService{repo: repo, transactions: transactions}
}

func (s *HistoryService) ListOrders(ctx context.Context, filter model.OrderHistoryFilter) ([]dto.OrderSummary, int64, error) {
	var orders []model.OrderHistory
	var total int64
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		orders, total, err = s.repo.ListOrders(ctx, tx, filter)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.OrderSummary, 0, len(orders))
	for _, order := range orders {
		items := make([]dto.OrderItemSnapshot, 0, len(order.Items))
		totalPortions := 0
		for _, item := range order.Items {
			items = append(items, historyItemSnapshot(item))
			totalPortions += item.Portions
		}
		result = append(result, dto.OrderSummary{ID: order.ID, Number: strconv.FormatInt(order.Number, 10),
			Status: "completed", CreatedAt: order.CreatedAt.UTC(),
			CreatedBy: dto.ActorBrief{ID: order.CreatedBy.ID, Name: order.CreatedBy.Name}, Items: items, TotalPortions: totalPortions})
	}
	return result, total, nil
}

func historyItemSnapshot(item model.OrderHistoryItem) dto.OrderItemSnapshot {
	version := item.RecipeVersion
	return dto.OrderItemSnapshot{DishID: item.DishID, DishName: item.DishName, Portions: item.Portions, RecipeVersion: &version}
}

func (s *HistoryService) GetOrder(ctx context.Context, id uuid.UUID) (dto.OrderDetail, error) {
	var order model.OrderHistory
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		order, err = s.repo.FindOrder(ctx, tx, id)
		return err
	})
	if err != nil {
		return dto.OrderDetail{}, err
	}
	result := dto.OrderDetail{ID: order.ID, Number: strconv.FormatInt(order.Number, 10), Status: "completed",
		CreatedAt: order.CreatedAt.UTC(), CreatedBy: dto.ActorBrief{ID: order.CreatedBy.ID, Name: order.CreatedBy.Name},
		Items: make([]dto.CreatedOrderItem, 0, len(order.Items))}
	for _, item := range order.Items {
		allocations := make([]dto.OrderAllocation, 0, len(item.Allocations))
		for _, a := range item.Allocations {
			allocations = append(allocations, dto.OrderAllocation{MovementID: a.MovementID, ProductID: a.ProductID,
				ProductName: a.ProductName, Unit: a.Unit, BatchID: a.BatchID, SupplyID: a.SupplyID,
				SupplyNumber: a.SupplyNumber, LineNumber: a.LineNumber, Quantity: a.Quantity,
				BatchRemainingQuantityAfter: a.RemainingQuantityAfter})
		}
		result.Items = append(result.Items, dto.CreatedOrderItem{ID: item.ID,
			OrderItemSnapshot: historyItemSnapshot(item), Allocations: allocations})
	}
	return result, nil
}
