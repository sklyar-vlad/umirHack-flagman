package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{ db repository.DBTX }

func NewRepository(db repository.DBTX) *Repository { return &Repository{db: db} }

func userDestinations(u *model.User) []any {
	return []any{&u.ID, &u.Name, &u.Email, &u.Role, &u.IsActive, &u.Revision, &u.CreatedAt, &u.LastLoginAt}
}

func (r *Repository) FindCredentials(ctx context.Context, email string) (model.Credentials, error) {
	var credentials model.Credentials
	destinations := append(userDestinations(&credentials.User), &credentials.PasswordHash)
	err := r.db.QueryRow(ctx, `SELECT id,name,email,role::text,is_active,revision,created_at,last_login_at,password_hash
 FROM users WHERE email=$1`, email).Scan(destinations...)
	if err != nil {
		return model.Credentials{}, authError("read credentials", err)
	}
	return credentials, nil
}

// A single statement serializes with user security updates, rechecks the password
// and active flag after lock waits, and commits last_login_at with the session.
func (r *Repository) CreateSession(
	ctx context.Context,
	userID uuid.UUID,
	passwordHash, tokenHash, csrf string,
) (model.Session, error) {
	var session model.Session
	destinations := append(userDestinations(&session.User), &session.CSRFToken, &session.ExpiresAt)
	err := r.db.QueryRow(ctx, `WITH employee AS (
 UPDATE users SET last_login_at=clock_timestamp()
 WHERE id=$1 AND password_hash=$2 AND is_active
 RETURNING id,name,email,role,is_active,revision,created_at,last_login_at
 ), issued AS (
 INSERT INTO sessions(user_id,token_hash,csrf_token,created_at,expires_at)
 SELECT id,$3,$4,last_login_at,last_login_at+interval '12 hours' FROM employee
 RETURNING user_id,csrf_token,expires_at
 ) SELECT u.id,u.name,u.email,u.role::text,u.is_active,u.revision,u.created_at,u.last_login_at,
 s.csrf_token,s.expires_at FROM employee u JOIN issued s ON s.user_id=u.id`,
		userID, passwordHash, tokenHash, csrf).Scan(destinations...)
	if err != nil {
		return model.Session{}, authError("issue session", err)
	}
	session.Actor = model.Actor{ID: session.User.ID, Role: session.User.Role}
	return session, nil
}

func (r *Repository) FindActiveSession(ctx context.Context, tokenHash string) (model.Session, error) {
	var session model.Session
	destinations := append(userDestinations(&session.User), &session.CSRFToken, &session.ExpiresAt,
		&session.Settings.RestaurantName, &session.Settings.Timezone, &session.Settings.ExpiringThresholdHours,
		&session.Settings.AutoWriteOffExpired, &session.Settings.Revision)
	err := r.db.QueryRow(ctx, `SELECT u.id,u.name,u.email,u.role::text,u.is_active,u.revision,u.created_at,u.last_login_at,
 s.csrf_token,s.expires_at,COALESCE(r.restaurant_name,''),COALESCE(r.timezone,'Europe/Moscow'),COALESCE(r.expiring_threshold_hours,24),
 COALESCE(r.auto_writeoff_expired,false),COALESCE(r.revision,1)
 FROM sessions s JOIN users u ON u.id=s.user_id LEFT JOIN restaurant_settings r ON r.id=1
 WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.is_active`, tokenHash).
		Scan(destinations...)
	if err != nil {
		return model.Session{}, authError("read active session", err)
	}
	session.Actor = model.Actor{ID: session.User.ID, Role: session.User.Role}
	return session, nil
}

func (r *Repository) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return authError("revoke session", err)
	}
	return nil
}

func authError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrUnauthorized
	}
	// PostgreSQL messages can contain values supplied by triggers or constraints.
	// Log only the operation and SQLSTATE, never credential-bearing details.
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		return fmt.Errorf("%s: database error (SQLSTATE %s)", operation, databaseError.Code)
	}
	return fmt.Errorf("%s: database operation failed", operation)
}
