package events

import (
	"context"

	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) TimeEventDue(ctx context.Context, q repository.DBTX) (bool, error) {
	var due bool
	err := q.QueryRow(ctx, `WITH due AS (
 UPDATE outbox_publication_state SET last_time_event_at=clock_timestamp()
 WHERE id=1 AND (last_time_event_at IS NULL OR last_time_event_at<=clock_timestamp()-interval '30 seconds')
 RETURNING id
 ) SELECT EXISTS(SELECT 1 FROM due)`).Scan(&due)
	return due, err
}
