package writeoff

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type HistoryRepository interface {
	List(context.Context, repository.DBTX, model.StockMovementFilter) ([]dto.StockMovement, int64, error)
}
type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type HistoryService struct {
	repo         HistoryRepository
	transactions Transactions
}

func NewHistoryService(repo HistoryRepository, transactions Transactions) *HistoryService {
	return &HistoryService{repo: repo, transactions: transactions}
}
func (s *HistoryService) List(ctx context.Context, f model.StockMovementFilter) ([]dto.StockMovement, int64, error) {
	actor, ok := requestctx.Actor(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, 0, apperrors.ErrUnauthorized
	}
	if actor.Role != "admin" && actor.Role != "manager" && actor.Role != "storekeeper" {
		return nil, 0, apperrors.ErrForbidden
	}
	var items []dto.StockMovement
	var total int64
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		items, total, err = s.repo.List(ctx, tx, f)
		return err
	})
	if err != nil {
		return nil, 0, safeError(err)
	}
	return items, total, nil
}
