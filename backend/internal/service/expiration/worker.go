package expiration

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"go.uber.org/zap"
)

const (
	pollInterval = 15 * time.Second
	passTimeout  = 30 * time.Second
)

type Settings interface {
	Get(context.Context, repository.DBTX, bool) (model.RestaurantSettings, error)
}
type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type Processor interface {
	Expire(context.Context) error
}

type Worker struct {
	settings     Settings
	transactions Transactions
	processor    Processor
	logger       *zap.Logger
}

func NewWorker(settings Settings, transactions Transactions, processor Processor, logger *zap.Logger) *Worker {
	return &Worker{settings: settings, transactions: transactions, processor: processor, logger: logger}
}

// Run performs an immediate pass, then polls well within the one-minute expiry SLA.
// The caller owns the goroutine and must cancel and join it before closing the DB.
func (w *Worker) Run(ctx context.Context) { w.run(ctx, pollInterval) }

func (w *Worker) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failed := false
	for {
		if ctx.Err() != nil {
			return
		}
		started := time.Now()
		passCtx, cancel := context.WithTimeout(ctx, passTimeout)
		err := w.pass(passCtx)
		cancel()
		telemetry.Default.Worker("expiration", started, err, ctx.Err() != nil, failed)
		failed = err != nil && ctx.Err() == nil
		if err != nil && ctx.Err() == nil {
			logFailure(w.logger, "expiry pass failed", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) pass(ctx context.Context) error {
	var enabled bool
	err := w.transactions.WithinTx(ctx, &pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		settings, err := w.settings.Get(ctx, tx, false)
		enabled = settings.AutoWriteOffExpired
		return err
	})
	if err != nil || !enabled {
		return err
	}
	return w.processor.Expire(ctx)
}

// Database messages can contain secrets. Emit only a safe operation and SQLSTATE.
func logFailure(logger *zap.Logger, operation string, err error) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		logger.Error(operation, zap.String("sqlstate", pg.Code))
		return
	}
	logger.Error(operation)
}
