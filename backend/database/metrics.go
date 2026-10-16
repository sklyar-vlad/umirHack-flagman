package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MetricsSnapshot(ctx context.Context, pool *pgxpool.Pool) (pending, age, expiryAge float64, err error) {
	err = pool.QueryRow(ctx, `WITH t AS (SELECT statement_timestamp() AS now)
 SELECT (SELECT count(*)::float8 FROM outbox_events WHERE published_at IS NULL),
 COALESCE((SELECT GREATEST(0,extract(epoch FROM t.now-min(created_at)))::float8 FROM outbox_events WHERE published_at IS NULL),0),
 CASE WHEN (SELECT auto_writeoff_expired FROM restaurant_settings WHERE id=1)
 THEN COALESCE((SELECT GREATEST(0,extract(epoch FROM t.now-min(expires_at)))::float8 FROM batches WHERE remaining_quantity>0 AND expires_at<=t.now),0) ELSE 0 END FROM t`).Scan(&pending, &age, &expiryAge)
	return
}
