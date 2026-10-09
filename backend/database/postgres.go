package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sklyar-vlad/umirHack-flagman/internal/config"
)

func NewPostgres(ctx context.Context, cfg config.ConfigDatabase) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
