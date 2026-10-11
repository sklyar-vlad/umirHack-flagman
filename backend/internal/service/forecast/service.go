package forecast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Consumption(context.Context, repository.DBTX, []uuid.UUID, time.Time, int) ([]model.ConsumptionTotal, error)
	Batches(context.Context, repository.DBTX, []uuid.UUID, time.Time) ([]model.OrderBatch, error)
}
type MenuRepository interface {
	Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error)
}
type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type Service struct {
	repo         Repository
	menu         MenuRepository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, menu MenuRepository, transactions Transactions) *Service {
	return &Service{repo: repo, menu: menu, transactions: transactions, now: time.Now}
}

func (s *Service) List(ctx context.Context, f model.ForecastFilter) (dto.ListResponse[dto.StopListForecast], error) {
	actor, ok := requestctx.Actor(ctx)
	if !ok || actor.ID == uuid.Nil {
		return dto.ListResponse[dto.StopListForecast]{}, apperrors.ErrUnauthorized
	}
	if actor.Role != "admin" && actor.Role != "manager" {
		return dto.ListResponse[dto.StopListForecast]{}, apperrors.ErrForbidden
	}
	if err := validateFilter(f); err != nil {
		return dto.ListResponse[dto.StopListForecast]{}, err
	}
	var dishes []model.Dish
	var batches []model.OrderBatch
	var totals []model.ConsumptionTotal
	var now time.Time
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		now = s.now().UTC()
		active := true
		var err error
		dishes, err = s.menu.Load(ctx, tx, &model.DishFilter{IsActive: &active}, now)
		if err != nil {
			return err
		}
		products := []uuid.UUID{}
		seen := map[uuid.UUID]bool{}
		for _, d := range dishes {
			for _, i := range d.Recipe {
				if !seen[i.ProductID] {
					products = append(products, i.ProductID)
					seen[i.ProductID] = true
				}
			}
		}
		batches, err = s.repo.Batches(ctx, tx, products, now)
		if err != nil {
			return err
		}
		totals, err = s.repo.Consumption(ctx, tx, products, now, f.LookbackDays)
		return err
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			err = fmt.Errorf("forecast: database error (SQLSTATE %s)", pg.Code)
		}
		return dto.ListResponse[dto.StopListForecast]{}, err
	}
	items, err := BuildForecasts(dishes, batches, totals, now, f)
	if err != nil {
		return dto.ListResponse[dto.StopListForecast]{}, err
	}
	total := int64(len(items))
	offset, _ := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	page := []dto.StopListForecast{}
	if offset < total {
		end := offset + int64(f.PageSize)
		if end < offset || end > total {
			end = total
		}
		page = items[offset:end]
	}
	return dto.ListResponse[dto.StopListForecast]{Items: page, Total: total, Page: f.Page, PageSize: f.PageSize, Meta: map[string]any{
		"server_time": now.Format(time.RFC3339Nano), "lookback_days": f.LookbackDays, "horizon_hours": f.HorizonHours,
	}}, nil
}
