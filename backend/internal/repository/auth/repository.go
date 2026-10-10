package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{ db repository.DBTX }

func NewRepository(db repository.DBTX) *Repository { return &Repository{db: db} }

func (r *Repository) FindActiveSession(ctx context.Context, tokenHash string) (model.Session, error) {
	const query = `
		SELECT u.id, u.role::text, s.csrf_token
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL
			AND s.expires_at > clock_timestamp() AND u.is_active
	`
	var session model.Session
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(&session.Actor.ID, &session.Actor.Role, &session.CSRFToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("read active session: %w", err)
	}
	return session, nil
}
