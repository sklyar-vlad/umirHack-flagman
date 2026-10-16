package product

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository interface {
	List(context.Context, repository.DBTX, model.ProductFilter) ([]model.Product, int64, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	transactions Transactions
}

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions}
}

func (s *Service) ListProducts(ctx context.Context, filter model.ProductFilter) ([]dto.Product, int64, error) {
	var products []model.Product
	var total int64
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := s.transactions.WithinTx(ctx, options, func(tx repository.DBTX) error {
		var err error
		products, total, err = s.repo.List(ctx, tx, filter)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.Product, 0, len(products))
	for i := range products {
		p := &products[i]
		items = append(items, dto.Product{
			ProductBrief:           dto.ProductBrief{ID: p.ID, Name: p.Name, Unit: p.Unit, DisplayUnit: p.DisplayUnit},
			Category:               dto.CategoryBrief{ID: p.Category.ID, Name: p.Category.Name},
			StandardShelfLifeHours: p.StandardShelfLifeHours, TargetStock: p.TargetStock,
			IsActive: p.IsActive, Revision: p.Revision, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return items, total, nil
}
