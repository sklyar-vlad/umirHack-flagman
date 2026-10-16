package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"go.uber.org/zap"
)

type Transactions interface {
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type PublicationRepository interface {
	Read(context.Context, repository.DBTX, int64) (model.EventPage, error)
	TimeEventDue(context.Context, repository.DBTX) (bool, error)
	Cleanup(context.Context, repository.DBTX) error
	Lock(context.Context, repository.DBTX) error
	Pending(context.Context, repository.DBTX) ([]uuid.UUID, error)
	Publish(context.Context, repository.DBTX, []uuid.UUID) error
}
type Service struct {
	repo         PublicationRepository
	transactions Transactions
	logger       *zap.Logger
}

func NewService(repo PublicationRepository, transactions Transactions, logger *zap.Logger) *Service {
	return &Service{repo: repo, transactions: transactions, logger: logger}
}

func (s *Service) Dispatch(ctx context.Context) error {
	return s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.Lock(ctx, tx); err != nil {
			return err
		}
		ids, err := s.repo.Pending(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.repo.Publish(ctx, tx, ids); err != nil {
			return err
		}
		return s.repo.Cleanup(ctx, tx)
	})
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	failed := false
	for {
		if ctx.Err() != nil {
			return
		}
		started := time.Now()
		pass, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.TimeTick(pass)
		if err == nil {
			err = s.Dispatch(pass)
		}
		cancel()
		telemetry.Default.Worker("outbox", started, err, ctx.Err() != nil, failed)
		failed = err != nil && ctx.Err() == nil
		if err != nil && ctx.Err() == nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) {
				s.logger.Error("outbox dispatch failed", zap.String("sqlstate", pg.Code))
			} else {
				s.logger.Error("outbox dispatch failed")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Read(ctx context.Context, after int64) (model.EventPage, error) {
	var page model.EventPage
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			page, err = s.repo.Read(ctx, tx, after)
			return err
		},
	)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			return model.EventPage{}, fmt.Errorf("read event stream: database error (SQLSTATE %s)", pg.Code)
		}
		return model.EventPage{}, errors.New("read event stream: database operation failed")
	}
	return page, nil
}

func (s *Service) TimeTick(ctx context.Context) error {
	return s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.Lock(ctx, tx); err != nil {
			return err
		}
		due, err := s.repo.TimeEventDue(ctx, tx)
		if err != nil || !due {
			return err
		}
		return s.transactions.AppendOutbox(ctx, tx, &model.OutboxEvent{Type: model.InventoryTimeChanged})
	})
}
