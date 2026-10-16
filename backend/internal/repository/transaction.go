package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"go.uber.org/zap"
)

const rollbackTimeout = 5 * time.Second

var ErrTransactionRequired = errors.New("an explicit transaction is required")

func RequireTransaction(q DBTX) (pgx.Tx, error) {
	tx, ok := q.(pgx.Tx)
	if !ok || tx == nil {
		return nil, ErrTransactionRequired
	}
	value := reflect.ValueOf(tx)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, ErrTransactionRequired
	}
	return tx, nil
}

func (r *repository) WithinTx(
	ctx context.Context,
	options *pgx.TxOptions,
	fn func(tx DBTX) error,
) (result error) {
	defer func() {
		var pg *pgconn.PgError
		if errors.As(result, &pg) {
			telemetry.Default.Conflict(pg.Code)
		}
	}()
	txOptions := pgx.TxOptions{}
	if options != nil {
		txOptions = *options
	}

	tx, err := r.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			rollbackTimeout,
		)
		defer cancel()

		rollbackErr := tx.Rollback(rollbackCtx)
		if rollbackErr != nil &&
			!errors.Is(rollbackErr, pgx.ErrTxClosed) {
			r.logger.Error(
				"failed to rollback transaction",
				zap.Error(rollbackErr),
			)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
