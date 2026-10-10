package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
)

type Repository struct{ *menurepository.Repository }

func NewRepository() *Repository { return &Repository{menurepository.NewRepository()} }

func (r *Repository) LockDishes(ctx context.Context, q repository.DBTX, ids []uuid.UUID) error {
	return lockRows(ctx, q, "dishes", ids)
}

func (r *Repository) LockProducts(ctx context.Context, q repository.DBTX, ids []uuid.UUID) error {
	return lockRows(ctx, q, "products", ids)
}

// table is selected only by the two methods above, never from user input.
func lockRows(ctx context.Context, q repository.DBTX, table string, ids []uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT id FROM "+table+" WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE", ids)
	if err != nil {
		return fmt.Errorf("lock %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *Repository) LockBatches(ctx context.Context, q repository.DBTX, ids []uuid.UUID) ([]model.OrderBatch, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	// Include expired rows: expiry must be evaluated only after every lock wait.
	rows, err := tx.Query(ctx, `SELECT b.id,b.product_id,b.supply_id,s.number,b.line_number,
		b.remaining_quantity::text,b.expires_at,s.received_at FROM batches b JOIN supplies s ON s.id=b.supply_id
		WHERE b.product_id=ANY($1::uuid[]) AND b.remaining_quantity>0
		ORDER BY b.id FOR UPDATE OF b`, ids)
	if err != nil {
		return nil, fmt.Errorf("lock order batches: %w", err)
	}
	defer rows.Close()
	batches := []model.OrderBatch{}
	for rows.Next() {
		var b model.OrderBatch
		if err := rows.Scan(
			&b.ID,
			&b.ProductID,
			&b.SupplyID,
			&b.SupplyNumber,
			&b.LineNumber,
			&b.RemainingQuantity,
			&b.ExpiresAt,
			&b.ReceivedAt,
		); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}
