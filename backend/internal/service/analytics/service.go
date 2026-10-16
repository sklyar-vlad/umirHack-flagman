package analytics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type Repository interface {
	ProductFlow(context.Context, repository.DBTX, model.AnalyticsFilter) ([]dto.ProductFlowAnalytics, int64, error)
	IngredientShortages(
		context.Context,
		repository.DBTX,
		model.AnalyticsFilter,
	) ([]dto.IngredientShortageAnalytics, int64, error)
	WriteOffs(context.Context, repository.DBTX, model.AnalyticsFilter) ([]dto.WriteOffAnalytics, int64, error)
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

func (s *Service) ProductFlow(ctx context.Context, f model.AnalyticsFilter) ([]dto.ProductFlowAnalytics, int64, error) {
	if err := authorize(ctx); err != nil {
		return nil, 0, err
	}
	var items []dto.ProductFlowAnalytics
	var total int64
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			items, total, err = s.repo.ProductFlow(ctx, tx, f)
			return err
		},
	)
	return items, total, safeError(err)
}

func authorize(ctx context.Context) error {
	actor, ok := requestctx.Actor(ctx)
	if !ok || actor.ID == uuid.Nil {
		return apperrors.ErrUnauthorized
	}
	if actor.Role != "admin" && actor.Role != "manager" {
		return apperrors.ErrForbidden
	}
	return nil
}

func safeError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("analytics: database error (SQLSTATE %s)", pg.Code)
	}
	return err
}

func (s *Service) WriteOffs(ctx context.Context, f model.AnalyticsFilter) ([]dto.WriteOffAnalytics, int64, error) {
	if err := authorize(ctx); err != nil {
		return nil, 0, err
	}
	var items []dto.WriteOffAnalytics
	var total int64
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			items, total, err = s.repo.WriteOffs(ctx, tx, f)
			return err
		},
	)
	if err != nil {
		return nil, 0, safeError(err)
	}
	return items, total, nil
}

func (s *Service) IngredientShortages(
	ctx context.Context,
	f model.AnalyticsFilter,
) ([]dto.IngredientShortageAnalytics, int64, error) {
	if err := authorize(ctx); err != nil {
		return nil, 0, err
	}
	var items []dto.IngredientShortageAnalytics
	var total int64
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			items, total, err = s.repo.IngredientShortages(ctx, tx, f)
			return err
		},
	)
	if err != nil {
		return nil, 0, safeError(err)
	}
	return items, total, nil
}
