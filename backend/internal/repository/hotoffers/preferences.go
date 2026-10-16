package hotoffers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) Preferences(
	ctx context.Context,
	q repository.DBTX,
) (map[uuid.UUID]model.HotOfferPreference, error) {
	rows, err := q.Query(ctx, `SELECT dish_id,is_published,is_manual,message,revision FROM hot_offer_preferences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[uuid.UUID]model.HotOfferPreference{}
	for rows.Next() {
		var id uuid.UUID
		var p model.HotOfferPreference
		if err := rows.Scan(&id, &p.Published, &p.Manual, &p.Message, &p.Revision); err != nil {
			return nil, err
		}
		result[id] = p
	}
	return result, rows.Err()
}

func (r *Repository) SavePreference(
	ctx context.Context,
	q repository.DBTX,
	id uuid.UUID,
	actor uuid.UUID,
	p model.HotOfferPreference,
	expected int,
) (model.HotOfferPreference, error) {
	var role string
	if err := q.QueryRow(ctx, `SELECT role::text FROM users WHERE id=$1 AND is_active FOR SHARE`, actor).
		Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, apperrors.ErrForbidden
		}
		return p, err
	}
	if role != "admin" && role != "manager" {
		return p, apperrors.ErrForbidden
	}
	var dish uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM dishes WHERE id=$1 FOR UPDATE`, id).Scan(&dish); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, apperrors.ErrNotFound
		}
		return p, err
	}
	var revision int
	err := q.QueryRow(ctx, `SELECT revision FROM hot_offer_preferences WHERE dish_id=$1`, id).Scan(&revision)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	if revision != expected {
		return p, &apperrors.RevisionConflict{CurrentRevision: revision}
	}
	err = q.QueryRow(ctx, `INSERT INTO hot_offer_preferences(dish_id,is_published,is_manual,message,updated_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(dish_id) DO UPDATE SET is_published=$2,is_manual=$3,message=$4,updated_by=$5,revision=hot_offer_preferences.revision+1,updated_at=clock_timestamp() RETURNING revision`, id, p.Published, p.Manual, p.Message, actor).
		Scan(&p.Revision)
	return p, err
}

func (r *Repository) Preference(
	ctx context.Context,
	q repository.DBTX,
	id uuid.UUID,
) (model.HotOfferPreference, error) {
	var p model.HotOfferPreference
	err := q.QueryRow(ctx, `SELECT COALESCE(p.is_published,true),COALESCE(p.is_manual,false),COALESCE(p.message,''),COALESCE(p.revision,0) FROM dishes d LEFT JOIN hot_offer_preferences p ON p.dish_id=d.id WHERE d.id=$1`, id).
		Scan(&p.Published, &p.Manual, &p.Message, &p.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.ErrNotFound
	}
	return p, err
}
