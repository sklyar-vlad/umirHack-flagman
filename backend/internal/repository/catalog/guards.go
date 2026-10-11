package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

// The caller holds FOR UPDATE on the product. Receipts and recipe writers must
// acquire the same product lock before using its current units/activity.
func (r *Repository) HasHistory(ctx context.Context, q repository.DBTX, id uuid.UUID) (bool, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return false, err
	}
	var used bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM batches WHERE product_id=$1) OR EXISTS(SELECT 1 FROM recipe_items WHERE product_id=$1)`, id).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("check catalog history: %w", err)
	}
	return used, nil
}

func (r *Repository) ActiveDishes(ctx context.Context, q repository.DBTX, id uuid.UUID) ([]apperrors.DishReference, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id,d.name FROM dishes d
 JOIN LATERAL(SELECT id FROM recipe_versions WHERE dish_id=d.id ORDER BY version DESC LIMIT 1) rv ON true
 WHERE d.is_active AND EXISTS(SELECT 1 FROM recipe_items ri WHERE ri.recipe_version_id=rv.id AND ri.product_id=$1)
 ORDER BY d.name,d.id`, id)
	if err != nil {
		return nil, fmt.Errorf("check active ingredient: %w", err)
	}
	defer rows.Close()
	dishes := make([]apperrors.DishReference, 0)
	for rows.Next() {
		var d apperrors.DishReference
		if err := rows.Scan(&d.ID, &d.Name); err != nil {
			return nil, err
		}
		dishes = append(dishes, d)
	}
	return dishes, rows.Err()
}
