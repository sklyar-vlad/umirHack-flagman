package supply

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) FindProducts(
	ctx context.Context,
	tx repository.DBTX,
	ids []uuid.UUID,
) (map[uuid.UUID]model.SupplyProduct, error) {
	if _, err := repository.RequireTransaction(tx); err != nil {
		return nil, err
	}
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}

	const query = `
		SELECT id, name, unit::text, is_active, standard_shelf_life_hours
		FROM products
		WHERE id = ANY($1::text[]::uuid[])
		ORDER BY id
		FOR UPDATE
	`

	rows, err := tx.Query(ctx, query, values)
	if err != nil {
		return nil, fmt.Errorf("find supply products: %w", err)
	}
	defer rows.Close()

	products := make(map[uuid.UUID]model.SupplyProduct, len(ids))

	for rows.Next() {
		var product model.SupplyProduct

		if err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Unit,
			&product.IsActive,
			&product.StandardShelfLifeHours,
		); err != nil {
			return nil, fmt.Errorf("scan supply product: %w", err)
		}

		products[product.ID] = product
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read supply products: %w", err)
	}

	return products, nil
}

func (r *Repository) NumberExists(
	ctx context.Context,
	tx repository.DBTX,
	number string,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM supplies WHERE number = $1
		)
	`

	var exists bool

	if err := tx.QueryRow(ctx, query, number).Scan(&exists); err != nil {
		return false, fmt.Errorf("check supply number: %w", err)
	}

	return exists, nil
}
