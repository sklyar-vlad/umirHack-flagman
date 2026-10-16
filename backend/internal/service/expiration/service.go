package expiration

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
)

type Repository interface {
	Candidates(context.Context, repository.DBTX) ([]uuid.UUID, error)
	LockExpired(context.Context, repository.DBTX, uuid.UUID) (*dto.StockMovement, error)
	WriteExpiration(context.Context, repository.DBTX, *dto.StockMovement) error
}
type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}
type Service struct {
	repo         Repository
	transactions Transactions
	events       EventWriter
	logger       *zap.Logger
}

func NewService(repo Repository, transactions Transactions, events EventWriter, logger *zap.Logger) *Service {
	return &Service{repo: repo, transactions: transactions, events: events, logger: logger}
}

func (s *Service) Expire(ctx context.Context) error {
	var ids []uuid.UUID
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		ids, err = s.repo.Candidates(ctx, tx)
		return err
	})
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.expireBatch(batchCtx, id)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logFailure(s.logger, "expire batch failed", err)
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) expireBatch(ctx context.Context, id uuid.UUID) error {
	return s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		m, err := s.repo.LockExpired(ctx, tx, id)
		if err != nil || m == nil {
			return err
		}
		if err := s.repo.WriteExpiration(ctx, tx, m); err != nil {
			return err
		}
		snapshot := &model.StockMovementAuditSnapshot{
			BatchID:                m.BatchID,
			Type:                   m.Type,
			DeltaQuantity:          m.DeltaQuantity,
			RemainingQuantityAfter: m.RemainingQuantityAfter,
			ProductNameSnapshot:    m.ProductNameSnapshot,
			UnitSnapshot:           m.UnitSnapshot,
		}
		if err := s.events.AppendAudit(
			ctx,
			tx,
			&model.AuditEntry{
				Action:     model.AuditExpiration,
				EntityType: model.AuditStockMovement,
				EntityID:   &m.ID,
				After:      snapshot,
			},
		); err != nil {
			return err
		}
		return s.events.AppendOutbox(
			ctx,
			tx,
			&model.OutboxEvent{
				Type:       model.InventoryChanged,
				ProductIDs: []uuid.UUID{m.ProductID},
				BatchIDs:   []uuid.UUID{m.BatchID},
			},
		)
	})
}
