package events

import (
	"context"

	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) Cleanup(ctx context.Context, q repository.DBTX) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH retention AS (
 SELECT clock_timestamp()-interval '24 hours' AS cutoff
 ), boundary AS (
 SELECT min(cursor) AS first_retained FROM outbox_events,retention
 WHERE published_at >= cutoff
 ), removed AS (
 DELETE FROM outbox_events USING boundary,retention
 WHERE published_at < cutoff
 AND (first_retained IS NULL OR cursor < first_retained)
 RETURNING cursor
 ) UPDATE outbox_publication_state
 SET retention_floor=GREATEST(retention_floor,COALESCE((SELECT max(cursor) FROM removed),0))
 WHERE id=1`)
	return err
}
