package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

const rollbackTimeout = 5 * time.Second

func (r *repository) WithinTx(
	ctx context.Context,
	options *pgx.TxOptions,
	fn func(tx DBTX) error,
) error {
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
