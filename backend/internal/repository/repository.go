package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

type db interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type repository struct {
	pool   db
	logger *zap.Logger
}

func NewRepository(pool db, logger *zap.Logger) *repository {
	return &repository{pool: pool, logger: logger}
}
