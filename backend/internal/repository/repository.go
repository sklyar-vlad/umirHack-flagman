package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type db interface {
	DBTX
	BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error)
}

type repository struct {
	pool   db
	logger *zap.Logger
}

func NewRepository(pool db, logger *zap.Logger) *repository {
	return &repository{
		pool:   pool,
		logger: logger,
	}
}

// func (r *repository) executor(tx DBTX) DBTX {
// 	if tx != nil {
// 		return tx
// 	}

// 	return r.pool
// }
