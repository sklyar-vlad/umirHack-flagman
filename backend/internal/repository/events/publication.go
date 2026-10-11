package events

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

const PageSize = 100

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Lock serializes publishers across processes. Business writers never acquire
// this advisory lock, and uncommitted business rows are invisible to Pending.
func (r *Repository) Lock(ctx context.Context, q repository.DBTX) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(240024,0)`)
	return err
}

func (r *Repository) Pending(ctx context.Context, q repository.DBTX) ([]uuid.UUID, error) {
	rows, err := q.Query(
		ctx,
		`SELECT id FROM outbox_events WHERE published_at IS NULL ORDER BY created_at,id LIMIT $1`,
		PageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("read pending outbox: %w", err)
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Each assignment is invisible until the serialized publication transaction
// commits. Sequence gaps from rollback are legal; cursor order never reverses.
func (r *Repository) Publish(ctx context.Context, q repository.DBTX, ids []uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	var high int64
	for _, id := range ids {
		if err := tx.QueryRow(ctx, `UPDATE outbox_events SET cursor=nextval('outbox_cursor_seq'),published_at=clock_timestamp() WHERE id=$1 AND published_at IS NULL RETURNING cursor`, id).
			Scan(&high); err != nil {
			return fmt.Errorf("publish outbox: %w", err)
		}
	}
	if len(ids) > 0 {
		_, err = tx.Exec(ctx, `UPDATE outbox_publication_state SET high_water=$1 WHERE id=1`, high)
	}
	return err
}
