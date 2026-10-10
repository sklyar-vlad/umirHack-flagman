package menu

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Load(context.Context, repository.DBTX, model.DishFilter, time.Time) ([]model.Dish, error)
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

func (s *Service) snapshot(ctx context.Context, filter model.DishFilter) ([]dto.Dish, time.Time, error) {
	var dishes []model.Dish
	var now time.Time
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		now = s.now().UTC()
		var err error
		dishes, err = s.repo.Load(ctx, tx, filter, now)
		return err
	})
	if err != nil {
		return nil, now, err
	}
	items := make([]dto.Dish, 0, len(dishes))
	for _, dish := range dishes {
		availability, err := CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return nil, now, fmt.Errorf("calculate availability for %s: %w", dish.ID, err)
		}
		if filter.Availability == "available" && !availability.IsAvailable {
			continue
		}
		if filter.Availability == "unavailable" && availability.IsAvailable {
			continue
		}
		recipe := make([]dto.RecipeIngredient, 0, len(dish.Recipe))
		for _, ingredient := range dish.Recipe {
			recipe = append(recipe, dto.RecipeIngredient{ProductID: ingredient.ProductID, ProductName: ingredient.ProductName, Unit: ingredient.Unit, QuantityPerPortion: ingredient.QuantityPerPortion})
		}
		items = append(items, dto.Dish{ID: dish.ID, Name: dish.Name, Description: dish.Description, IsActive: dish.IsActive,
			Revision: dish.Revision, RecipeVersion: dish.RecipeVersion, Recipe: recipe, Availability: availability,
			Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)},
		})
	}
	return items, now, nil
}

func (s *Service) ListDishes(ctx context.Context, filter model.DishFilter) (dto.DishListResponse, error) {
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return dto.DishListResponse{}, err
	}
	items, now, err := s.snapshot(ctx, filter)
	if err != nil {
		return dto.DishListResponse{}, err
	}
	total := int64(len(items))
	page := []dto.Dish{}
	if offset < total {
		end := offset + int64(filter.PageSize)
		if end < offset || end > total {
			end = total
		}
		page = items[offset:end]
	}
	return dto.DishListResponse{Items: page, Total: total, Page: filter.Page, PageSize: filter.PageSize,
		Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)}}, nil
}

func (s *Service) GetDish(ctx context.Context, id uuid.UUID) (dto.Dish, error) {
	items, _, err := s.snapshot(ctx, model.DishFilter{ID: &id})
	if err != nil {
		return dto.Dish{}, err
	}
	if len(items) == 0 {
		return dto.Dish{}, apperrors.ErrNotFound
	}
	return items[0], nil
}

func (s *Service) StopList(ctx context.Context, filter model.DishFilter) (dto.DishListResponse, error) {
	active := true
	filter.IsActive = &active
	filter.Availability = "unavailable"
	return s.ListDishes(ctx, filter)
}
