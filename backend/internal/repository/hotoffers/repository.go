package hotoffers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) Load(
	ctx context.Context,
	q repository.DBTX,
	products []uuid.UUID,
	now time.Time,
) ([]model.OrderBatch, int, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	var hours int
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT expiring_threshold_hours FROM restaurant_settings WHERE id=1),24)`).
		Scan(&hours); err != nil {
		return nil, 0, fmt.Errorf("read hot offers threshold: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT b.id,b.product_id,b.supply_id,s.number,b.line_number,
		b.remaining_quantity::text,b.expires_at,s.received_at
		FROM batches b JOIN supplies s ON s.id=b.supply_id
		WHERE b.product_id=ANY($1::uuid[]) AND b.remaining_quantity>0 AND b.expires_at>$2
		ORDER BY b.expires_at,s.received_at,b.id`, products, now.UTC())
	if err != nil {
		return nil, 0, fmt.Errorf("load hot offers batches: %w", err)
	}
	defer rows.Close()
	batches := []model.OrderBatch{}
	for rows.Next() {
		var b model.OrderBatch
		if err := rows.Scan(&b.ID, &b.ProductID, &b.SupplyID, &b.SupplyNumber, &b.LineNumber,
			&b.RemainingQuantity, &b.ExpiresAt, &b.ReceivedAt); err != nil {
			return nil, 0, fmt.Errorf("scan hot offers batch: %w", err)
		}
		batches = append(batches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read hot offers batches: %w", err)
	}
	return batches, hours, nil
}
