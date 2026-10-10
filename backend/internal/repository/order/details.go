package order

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

func (r *Repository) FindOrder(ctx context.Context, q repository.DBTX, id uuid.UUID) (model.OrderHistory, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.OrderHistory{}, err
	}
	var result model.OrderHistory
	err = tx.QueryRow(ctx, `SELECT o.id,o.number,o.created_at,u.id,u.name
		FROM orders o JOIN users u ON u.id=o.created_by WHERE o.id=$1`, id).
		Scan(&result.ID, &result.Number, &result.CreatedAt, &result.CreatedBy.ID, &result.CreatedBy.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.OrderHistory{}, apperrors.ErrNotFound
	}
	if err != nil {
		return model.OrderHistory{}, fmt.Errorf("read order: %w", err)
	}
	items, err := r.historyItems(ctx, tx, []uuid.UUID{id})
	if err != nil {
		return model.OrderHistory{}, err
	}
	result.Items = items[id]
	positions := make(map[uuid.UUID]int, len(result.Items))
	for i := range result.Items {
		positions[result.Items[i].ID] = i
		result.Items[i].Allocations = make([]model.OrderHistoryAllocation, 0)
	}
	// Product labels and balances come from the immutable movement, never the
	// current product or batch balance. Batch/supply joins provide source IDs.
	rows, err := tx.Query(ctx, `SELECT sm.order_item_id,sm.id,b.product_id,
		sm.product_name_snapshot,sm.unit_snapshot::text,b.id,s.id,s.number,b.line_number,
		(-sm.delta_quantity)::text,sm.remaining_quantity_after::text
		FROM stock_movements sm JOIN order_items oi ON oi.id=sm.order_item_id
		JOIN batches b ON b.id=sm.batch_id JOIN supplies s ON s.id=b.supply_id
		WHERE oi.order_id=$1 AND sm.type='order_consumption'
		ORDER BY oi.dish_id,sm.product_name_snapshot,b.product_id,b.expires_at,s.received_at,b.id,sm.id`, id)
	if err != nil {
		return model.OrderHistory{}, fmt.Errorf("read order allocations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemID uuid.UUID
		var allocation model.OrderHistoryAllocation
		if err := rows.Scan(
			&itemID,
			&allocation.MovementID,
			&allocation.ProductID,
			&allocation.ProductName,
			&allocation.Unit,
			&allocation.BatchID,
			&allocation.SupplyID,
			&allocation.SupplyNumber,
			&allocation.LineNumber,
			&allocation.Quantity,
			&allocation.RemainingQuantityAfter,
		); err != nil {
			return model.OrderHistory{}, fmt.Errorf("scan order allocation: %w", err)
		}
		i, exists := positions[itemID]
		if !exists {
			return model.OrderHistory{}, fmt.Errorf("order movement references missing historical item")
		}
		result.Items[i].Allocations = append(result.Items[i].Allocations, allocation)
	}
	if err := rows.Err(); err != nil {
		return model.OrderHistory{}, fmt.Errorf("iterate order allocations: %w", err)
	}
	return result, nil
}
