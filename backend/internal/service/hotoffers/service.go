package hotoffers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Load(context.Context, repository.DBTX, []uuid.UUID, time.Time) ([]model.OrderBatch, int, error)
}

type MenuRepository interface {
	Load(context.Context, repository.DBTX, model.DishFilter, time.Time) ([]model.Dish, error)
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

func (s *Service) List(ctx context.Context, filter model.HotOffersFilter) (dto.HotOffersListResponse, error) {
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	var dishes []model.Dish
	var batches []model.OrderBatch
	var hours int
	var now time.Time
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		now = s.now().UTC()
		active := true
		var err error
		dishes, err = s.menu.Load(ctx, tx, model.DishFilter{IsActive: &active, Search: filter.Search}, now)
		if err != nil {
			return err
		}
		batches, hours, err = s.repo.Load(ctx, tx, ingredientProducts(dishes), now)
		return err
	})
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	offers, err := simulate(dishes, batches, now, hours)
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	scored, err := scoreOffers(offers, now, hours)
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	total := int64(len(scored))
	items := []dto.HotOffer{}
	if offset < total {
		end := offset + int64(filter.PageSize)
		if end < offset || end > total {
			end = total
		}
		for i := offset; i < end; i++ {
			items = append(items, renderOffer(scored[i], i+1, now))
		}
	}
	return dto.HotOffersListResponse{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize, Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)}}, nil
}

func ingredientProducts(dishes []model.Dish) []uuid.UUID {
	products := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, dish := range dishes {
		for _, ingredient := range dish.Recipe {
			if !seen[ingredient.ProductID] {
				products = append(products, ingredient.ProductID)
				seen[ingredient.ProductID] = true
			}
		}
	}
	return products
}
