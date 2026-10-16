package forecast

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

func (r *Repository) Consumption(
	ctx context.Context,
	q repository.DBTX,
	products []uuid.UUID,
	now time.Time,
	days int,
) ([]model.ConsumptionTotal, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT b.product_id,m.unit_snapshot::text,(-SUM(m.delta_quantity))::text
		FROM stock_movements m JOIN batches b ON b.id=m.batch_id
		WHERE m.type='order_consumption' AND b.product_id=ANY($1::uuid[])
		AND m.occurred_at >= $2 AND m.occurred_at < $3
		GROUP BY b.product_id,m.unit_snapshot ORDER BY b.product_id,m.unit_snapshot`,
		products, now.UTC().Add(-time.Duration(days)*24*time.Hour), now.UTC())
	if err != nil {
		return nil, fmt.Errorf("read forecast consumption: %w", err)
	}
	defer rows.Close()
	items := []model.ConsumptionTotal{}
	for rows.Next() {
		var v model.ConsumptionTotal
		if err := rows.Scan(&v.ProductID, &v.Unit, &v.Quantity); err != nil {
			return nil, fmt.Errorf("scan forecast consumption: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r *Repository) Batches(
	ctx context.Context,
	q repository.DBTX,
	products []uuid.UUID,
	now time.Time,
) ([]model.OrderBatch, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT b.id,b.product_id,b.remaining_quantity::text,b.expires_at,s.received_at
		FROM batches b JOIN supplies s ON s.id=b.supply_id
		WHERE b.product_id=ANY($1::uuid[]) AND b.remaining_quantity>0 AND b.expires_at>$2
		ORDER BY b.expires_at,s.received_at,b.id`, products, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("read forecast batches: %w", err)
	}
	defer rows.Close()
	items := []model.OrderBatch{}
	for rows.Next() {
		var v model.OrderBatch
		if err := rows.Scan(&v.ID, &v.ProductID, &v.RemainingQuantity, &v.ExpiresAt, &v.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan forecast batch: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
