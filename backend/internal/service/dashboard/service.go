package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	batchservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/batch"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/hotoffers"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/procurement"
)

type Repository interface {
	Load(context.Context, repository.DBTX, time.Time) (model.DashboardData, error)
	LoadRecipes(context.Context, repository.DBTX) ([]model.Dish, error)
	LoadBatches(context.Context, repository.DBTX) ([]model.OrderBatch, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions, now: time.Now}
}

func (s *Service) Get(ctx context.Context, role string) (dto.DashboardResponse, error) {
	switch role {
	case "admin", "manager", "storekeeper", "order_operator":
	default:
		return dto.DashboardResponse{}, apperrors.ErrForbidden
	}
	var data model.DashboardData
	var dishes []model.Dish
	var batches []model.OrderBatch
	var now time.Time
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			now = s.now().UTC()
			var err error
			data, err = s.repo.Load(ctx, tx, now)
			if err != nil {
				return err
			}
			dishes, err = s.repo.LoadRecipes(ctx, tx)
			if err != nil {
				return err
			}
			batches, err = s.repo.LoadBatches(ctx, tx)
			return err
		},
	)
	if err != nil {
		return dto.DashboardResponse{}, err
	}
	result := dto.DashboardResponse{
		ExpiringBatchesCount:       data.ExpiringBatchesCount,
		ExpiredBatchesCount:        data.ExpiredBatchesCount,
		AttentionTotal:             data.ExpiringBatchesCount + data.ExpiredBatchesCount,
		AttentionBatches:           []dto.AttentionBatch{},
		HotOffers:                  []dto.HotOffer{},
		ProcurementRecommendations: []dto.ProcurementItem{},
		Settings: dto.DashboardSettings{
			RestaurantName:         data.Settings.RestaurantName,
			Timezone:               data.Settings.Timezone,
			ExpiringThresholdHours: data.Settings.ExpiringThresholdHours,
		},
		Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)},
	}
	stockByProduct := make(map[uuid.UUID]model.ProcurementStock, len(data.Stock))
	for i := range data.Stock {
		stock := &data.Stock[i]
		stockByProduct[stock.Product.ID] = *stock
		available, err := quantity.ParseMilli(stock.AvailableQuantity)
		if err != nil {
			return dto.DashboardResponse{}, err
		}
		if available.Sign() == 0 {
			result.OutOfStockProductsCount++
		}
	}
	for i := range dishes {
		dish := &dishes[i]
		for j := range dish.Recipe {
			ingredient := &dish.Recipe[j]
			stock, ok := stockByProduct[ingredient.ProductID]
			if !ok {
				return dto.DashboardResponse{}, fmt.Errorf("dashboard stock missing for %s", ingredient.ProductID)
			}
			ingredient.AvailableQuantity = stock.AvailableQuantity
			ingredient.ExpiredQuantity = stock.ExpiredQuantity
		}
		availability, err := menu.CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return dto.DashboardResponse{}, err
		}
		if dish.IsActive && !availability.IsAvailable {
			result.StopListDishesCount++
		}
	}
	warningAt := now.Add(time.Duration(data.Settings.ExpiringThresholdHours) * time.Hour)
	for i := range batches {
		b := &batches[i]
		if b.ExpiresAt.After(warningAt) || len(result.AttentionBatches) == 20 {
			continue
		}
		stock, ok := stockByProduct[b.ProductID]
		if !ok {
			return dto.DashboardResponse{}, fmt.Errorf("dashboard stock missing for %s", b.ProductID)
		}
		status, seconds := batchservice.CalculateStatus(
			b.RemainingQuantity,
			b.ExpiresAt,
			now,
			data.Settings.ExpiringThresholdHours,
		)
		result.AttentionBatches = append(result.AttentionBatches, dto.AttentionBatch{
			BatchID: b.ID, ProductID: b.ProductID, ProductName: stock.Product.Name,
			SupplyID: b.SupplyID, SupplyNumber: b.SupplyNumber, LineNumber: b.LineNumber,
			RemainingQuantity: b.RemainingQuantity, Unit: stock.Product.Unit, Status: status,
			ExpiresAt: b.ExpiresAt.UTC().Format(time.RFC3339Nano), RemainingSeconds: seconds,
		})
	}
	if role != "storekeeper" {
		offers, err := hotoffers.BuildOffers(dishes, batches, now, data.Settings.ExpiringThresholdHours)
		if err != nil {
			return dto.DashboardResponse{}, err
		}
		offers, err = hotoffers.ApplyPreferences(offers, dishes, data.Preferences, role)
		if err != nil {
			return dto.DashboardResponse{}, err
		}
		result.HotOffers = offers[:min(3, len(offers))]
	}
	if role != "order_operator" {
		items, err := procurement.BuildRecommendations(data.Stock, dishes)
		if err != nil {
			return dto.DashboardResponse{}, err
		}
		result.ProcurementRecommendations = items[:min(3, len(items))]
	}
	return result, nil
}
