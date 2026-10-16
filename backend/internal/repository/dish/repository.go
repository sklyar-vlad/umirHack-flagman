package dish

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		return fmt.Errorf("lock dish actor: %w", err)
	}
	if !active || role != "admin" {
		return apperrors.ErrForbidden
	}
	return nil
}

func (r *Repository) LockDish(ctx context.Context, q repository.DBTX, id uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	var locked uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM dishes WHERE id=$1 FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock dish: %w", err)
	}
	return nil
}

func (r *Repository) Products(
	ctx context.Context,
	q repository.DBTX,
	ids []uuid.UUID,
	now time.Time,
	lock bool,
) ([]model.RecipeProduct, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	if lock {
		rows, err := tx.Query(ctx, `SELECT id FROM products WHERE id=ANY($1::uuid[]) ORDER BY id FOR SHARE`, ids)
		if err != nil {
			return nil, fmt.Errorf("lock recipe products: %w", err)
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	rows, err := tx.Query(ctx, `SELECT p.id,p.name,p.unit::text,p.is_active,
		COALESCE(SUM(b.remaining_quantity) FILTER (WHERE b.expires_at>$2),0.000)::text,
		COALESCE(SUM(b.remaining_quantity) FILTER (WHERE b.expires_at<=$2),0.000)::text
		FROM products p LEFT JOIN batches b ON b.product_id=p.id AND b.remaining_quantity>0
		WHERE p.id=ANY($1::uuid[]) GROUP BY p.id ORDER BY p.name,p.id`, ids, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("load recipe products: %w", err)
	}
	defer rows.Close()
	result := make([]model.RecipeProduct, 0)
	for rows.Next() {
		var p model.RecipeProduct
		if err := rows.Scan(
			&p.ProductID,
			&p.ProductName,
			&p.Unit,
			&p.IsActive,
			&p.AvailableQuantity,
			&p.ExpiredQuantity,
		); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *Repository) Create(ctx context.Context, q repository.DBTX, d *model.Dish) (uuid.UUID, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO dishes(name,description,is_active) VALUES($1,$2,$3) RETURNING id`, d.Name, d.Description, d.IsActive).
		Scan(&id)
	return id, err
}

func (r *Repository) AddRecipe(
	ctx context.Context,
	q repository.DBTX,
	dishID, actorID uuid.UUID,
	recipe []model.RecipeIngredient,
) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	var versionID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO recipe_versions(dish_id,version,created_by)
		SELECT $1,COALESCE(MAX(version),0)+1,$2 FROM recipe_versions WHERE dish_id=$1 RETURNING id`, dishID, actorID).Scan(&versionID)
	if err != nil {
		return err
	}
	for _, item := range recipe {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,$3::numeric)`,
			versionID,
			item.ProductID,
			item.QuantityPerPortion,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, q repository.DBTX, d *model.Dish) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		`UPDATE dishes SET name=$2,description=$3,is_active=$4,revision=revision+1,updated_at=now() WHERE id=$1`,
		d.ID,
		d.Name,
		d.Description,
		d.IsActive,
	)
	return err
}
