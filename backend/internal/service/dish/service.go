package dish

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
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
)

type Repository interface {
	LockActor(context.Context, repository.DBTX, uuid.UUID) error
	LockDish(context.Context, repository.DBTX, uuid.UUID) error
	Products(context.Context, repository.DBTX, []uuid.UUID, time.Time, bool) ([]model.RecipeProduct, error)
	Create(context.Context, repository.DBTX, model.Dish) (uuid.UUID, error)
	Update(context.Context, repository.DBTX, model.Dish) error
	AddRecipe(context.Context, repository.DBTX, uuid.UUID, uuid.UUID, []model.RecipeIngredient) error
}

type MenuReader interface {
	Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}

type Service struct {
	repo         Repository
	menu         MenuReader
	transactions Transactions
	events       EventWriter
	now          func() time.Time
}

func NewService(repo Repository, menu MenuReader, transactions Transactions, events EventWriter) *Service {
	return &Service{repo: repo, menu: menu, transactions: transactions, events: events, now: time.Now}
}

func actor(ctx context.Context) (model.Actor, error) {
	a, ok := requestctx.Actor(ctx)
	if !ok || a.ID == uuid.Nil {
		return a, apperrors.ErrUnauthorized
	}
	if a.Role != "admin" {
		return a, apperrors.ErrForbidden
	}
	return a, nil
}

func databaseError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" && pg.ConstraintName == "dishes_name_ci_uidx" {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("dish transaction: database error (SQLSTATE %s)", pg.Code)
	}
	return err
}

func (s *Service) load(ctx context.Context, tx repository.DBTX, id uuid.UUID, now time.Time) (model.Dish, error) {
	items, err := s.menu.Load(ctx, tx, &model.DishFilter{ID: &id}, now)
	if err != nil {
		return model.Dish{}, err
	}
	if len(items) != 1 {
		return model.Dish{}, apperrors.ErrNotFound
	}
	return items[0], nil
}

func dishDTO(d model.Dish, now time.Time) (dto.Dish, error) {
	a, err := menu.CalculateAvailability(d.IsActive, d.Recipe)
	if err != nil {
		return dto.Dish{}, err
	}
	result := dto.Dish{ID: d.ID, Name: d.Name, Description: d.Description, IsActive: d.IsActive, Revision: d.Revision, RecipeVersion: d.RecipeVersion, Availability: a, Recipe: []dto.RecipeIngredient{}, Meta: map[string]any{"server_time": now.UTC().Format(time.RFC3339Nano)}}
	for _, item := range d.Recipe {
		result.Recipe = append(result.Recipe, dto.RecipeIngredient{ProductID: item.ProductID, ProductName: item.ProductName, Unit: item.Unit, QuantityPerPortion: item.QuantityPerPortion})
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, input dto.CreateDishRequest) (dto.Dish, error) {
	a, err := actor(ctx)
	if err != nil {
		return dto.Dish{}, err
	}
	d := model.Dish{Name: input.Name, Description: input.Description, IsActive: true}
	if input.IsActive != nil {
		d.IsActive = *input.IsActive
	}
	if err := validateDish(&d); err != nil {
		return dto.Dish{}, err
	}
	recipe, err := validateRecipe(input.Recipe)
	if err != nil {
		return dto.Dish{}, err
	}
	var result dto.Dish
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.LockActor(ctx, tx, a.ID); err != nil {
			return err
		}
		if _, err := s.recipe(ctx, tx, recipe, s.now().UTC(), true); err != nil {
			return err
		}
		id, err := s.repo.Create(ctx, tx, d)
		if err != nil {
			return err
		}
		if err := s.repo.AddRecipe(ctx, tx, id, a.ID, recipe); err != nil {
			return err
		}
		now := s.now().UTC()
		after, err := s.load(ctx, tx, id, now)
		if err != nil {
			return err
		}
		result, err = dishDTO(after, now)
		if err != nil {
			return err
		}
		return s.changed(ctx, tx, a, nil, after)
	})
	if err != nil {
		return dto.Dish{}, databaseError(err)
	}
	return result, nil
}

func (s *Service) recipe(ctx context.Context, tx repository.DBTX, input []model.RecipeIngredient, now time.Time, lock bool) ([]model.RecipeIngredient, error) {
	ids := make([]uuid.UUID, 0, len(input))
	quantities := make(map[uuid.UUID]string, len(input))
	for _, item := range input {
		ids = append(ids, item.ProductID)
		quantities[item.ProductID] = item.QuantityPerPortion
	}
	products, err := s.repo.Products(ctx, tx, ids, now, lock)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]model.RecipeProduct, len(products))
	for _, p := range products {
		byID[p.ProductID] = p
	}
	for i, item := range input {
		p, ok := byID[item.ProductID]
		if !ok || !p.IsActive {
			return nil, field(fmt.Sprintf("recipe[%d].product_id", i))
		}
		if p.Unit == "pcs" && !wholeQuantity(item.QuantityPerPortion) {
			return nil, field(fmt.Sprintf("recipe[%d].quantity_per_portion", i))
		}
	}
	result := make([]model.RecipeIngredient, 0, len(products))
	for _, p := range products {
		p.QuantityPerPortion = quantities[p.ProductID]
		result = append(result, p.RecipeIngredient)
	}
	return result, nil
}
