package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

// State and page are read inside the caller's repeatable-read snapshot so
// cleanup/publication cannot change the replay boundary between these reads.
func (r *Repository) Read(ctx context.Context, q repository.DBTX, after int64) (model.EventPage, error) {
	page := model.EventPage{Events: []model.PublishedEvent{}}
	if err := q.QueryRow(ctx, `SELECT high_water,retention_floor FROM outbox_publication_state WHERE id=1`).
		Scan(&page.HighWater, &page.RetentionFloor); err != nil {
		return page, err
	}
	rows, err := q.Query(
		ctx,
		`SELECT cursor,event_type,payload FROM outbox_events WHERE published_at IS NOT NULL AND cursor>$1 AND cursor<=$2 ORDER BY cursor LIMIT $3`,
		after,
		page.HighWater,
		PageSize,
	)
	if err != nil {
		return page, fmt.Errorf("read published outbox: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e model.PublishedEvent
		var data []byte
		if err := rows.Scan(&e.Cursor, &e.Type, &data); err != nil {
			return page, err
		}
		if err := json.Unmarshal(data, &e.Payload); err != nil {
			return page, fmt.Errorf("decode published outbox: %w", err)
		}
		page.Events = append(page.Events, e)
	}
	return page, rows.Err()
}
