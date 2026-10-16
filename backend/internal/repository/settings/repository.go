package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) LockActor(ctx context.Context, q repository.DBTX, id uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	var active bool
	var role string
	err = tx.QueryRow(ctx, `SELECT is_active,role::text FROM users WHERE id=$1 FOR SHARE`, id).Scan(&active, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock settings actor: %w", err)
	}
	if !active || role != "admin" {
		return apperrors.ErrForbidden
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, q repository.DBTX, lock bool) (model.RestaurantSettings, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.RestaurantSettings{}, err
	}
	query := `SELECT restaurant_name,timezone,expiring_threshold_hours,auto_writeoff_expired,revision FROM restaurant_settings WHERE id=1`
	if lock {
		query += " FOR UPDATE"
	}
	var s model.RestaurantSettings
	err = tx.QueryRow(ctx, query).
		Scan(&s.RestaurantName, &s.Timezone, &s.ExpiringThresholdHours, &s.AutoWriteOffExpired, &s.Revision)
	if err != nil {
		return s, fmt.Errorf("get settings: %w", err)
	}
	return s, nil
}

func (r *Repository) Update(
	ctx context.Context,
	q repository.DBTX,
	s model.RestaurantSettings,
) (model.RestaurantSettings, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.RestaurantSettings{}, err
	}
	err = tx.QueryRow(ctx, `UPDATE restaurant_settings SET restaurant_name=$1,timezone=$2,expiring_threshold_hours=$3,auto_writeoff_expired=$4,revision=revision+1,updated_at=clock_timestamp() WHERE id=1 RETURNING revision`, s.RestaurantName, s.Timezone, s.ExpiringThresholdHours, s.AutoWriteOffExpired).
		Scan(&s.Revision)
	if err != nil {
		return s, fmt.Errorf("update settings: %w", err)
	}
	return s, nil
}
