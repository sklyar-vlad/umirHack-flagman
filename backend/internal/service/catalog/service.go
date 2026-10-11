package catalog

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type Repository interface {
	HasHistory(context.Context, repository.DBTX, uuid.UUID) (bool, error)
	ActiveDishes(context.Context, repository.DBTX, uuid.UUID) ([]apperrors.DishReference, error)
	LockActor(context.Context, repository.DBTX, uuid.UUID) error
	ListCategories(context.Context, repository.DBTX, model.CategoryFilter) ([]model.CatalogCategory, int64, error)
	GetCategory(context.Context, repository.DBTX, uuid.UUID, string) (model.CatalogCategory, error)
	CreateCategory(context.Context, repository.DBTX, string) (model.CatalogCategory, error)
	UpdateCategory(context.Context, repository.DBTX, model.CatalogCategory) (model.CatalogCategory, error)
	GetProduct(context.Context, repository.DBTX, uuid.UUID, bool) (model.Product, error)
	CreateProduct(context.Context, repository.DBTX, model.Product) (model.Product, error)
	UpdateProduct(context.Context, repository.DBTX, model.Product) (model.Product, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	events       EventWriter
}

func NewService(repo Repository, transactions Transactions, events EventWriter) *Service {
	return &Service{repo: repo, transactions: transactions, events: events}
}

func actor(ctx context.Context, write bool) (model.Actor, error) {
	a, ok := requestctx.Actor(ctx)
	if !ok || a.ID == uuid.Nil {
		return a, apperrors.ErrUnauthorized
	}
	if write && a.Role != "admin" {
		return a, apperrors.ErrForbidden
	}
	if a.Role != "admin" && a.Role != "manager" && a.Role != "storekeeper" && a.Role != "order_operator" {
		return a, apperrors.ErrForbidden
	}
	return a, nil
}

func categoryDTO(c model.CatalogCategory) dto.CatalogCategory {
	return dto.CatalogCategory{ID: c.ID, Name: c.Name, Revision: c.Revision}
}

func productDTO(p model.Product) dto.Product {
	return dto.Product{
		ProductBrief:           dto.ProductBrief{ID: p.ID, Name: p.Name, Unit: p.Unit, DisplayUnit: p.DisplayUnit},
		Category:               dto.CategoryBrief{ID: p.Category.ID, Name: p.Category.Name},
		StandardShelfLifeHours: p.StandardShelfLifeHours, TargetStock: p.TargetStock,
		IsActive: p.IsActive, Revision: p.Revision, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (s *Service) read(ctx context.Context, fn func(repository.DBTX) error) error {
	if _, err := actor(ctx, false); err != nil {
		return err
	}
	return databaseError(s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, fn))
}

func (s *Service) write(ctx context.Context, fn func(repository.DBTX) error) error {
	a, err := actor(ctx, true)
	if err != nil {
		return err
	}
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.LockActor(ctx, tx, a.ID); err != nil {
			return err
		}
		return fn(tx)
	})
	return mutationError(err)
}

func (s *Service) ListCategories(ctx context.Context, f model.CategoryFilter) ([]dto.CatalogCategory, int64, error) {
	var items []model.CatalogCategory
	var total int64
	err := s.read(ctx, func(tx repository.DBTX) error {
		var err error
		items, total, err = s.repo.ListCategories(ctx, tx, f)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.CatalogCategory, 0, len(items))
	for _, c := range items {
		result = append(result, categoryDTO(c))
	}
	return result, total, nil
}

func (s *Service) GetCategory(ctx context.Context, id uuid.UUID) (dto.CatalogCategory, error) {
	var c model.CatalogCategory
	err := s.read(ctx, func(tx repository.DBTX) error {
		var err error
		c, err = s.repo.GetCategory(ctx, tx, id, "")
		return err
	})
	if err != nil {
		return dto.CatalogCategory{}, err
	}
	return categoryDTO(c), nil
}

func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (dto.Product, error) {
	var p model.Product
	err := s.read(ctx, func(tx repository.DBTX) error {
		var err error
		p, err = s.repo.GetProduct(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return dto.Product{}, err
	}
	return productDTO(p), nil
}
