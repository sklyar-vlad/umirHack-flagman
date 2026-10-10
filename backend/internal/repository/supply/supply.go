package supply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) InsertSupply(
	ctx context.Context,
	tx repository.DBTX,
	supply model.NewSupply,
) (time.Time, error) {
	if _, err := repository.RequireTransaction(tx); err != nil {
		return time.Time{}, err
	}

	const query = `
		INSERT INTO supplies (id, number, received_at, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`
	var createdAt time.Time
	err := tx.QueryRow(ctx, query,
		supply.ID.String(), supply.Number, supply.ReceivedAt, supply.CreatedBy.String(),
	).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		// Different products need not share locks. The unique constraint is the
		// final authority if concurrent supplies pass the number precheck.
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "supplies_number_key" {
			return time.Time{}, fmt.Errorf("%w: supply number already exists", apperrors.ErrConflict)
		}
		return time.Time{}, fmt.Errorf("insert supply: %w", err)
	}
	return createdAt.UTC(), nil
}
