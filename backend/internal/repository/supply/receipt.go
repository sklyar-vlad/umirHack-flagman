package supply

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) InsertReceipt(
	ctx context.Context,
	tx repository.DBTX,
	receipt *model.NewSupplyReceipt,
) error {
	if _, err := repository.RequireTransaction(tx); err != nil {
		return err
	}
	if receipt == nil || receipt.BatchID == uuid.Nil || receipt.ActorID == uuid.Nil ||
		receipt.QuantityMilli < 1 || receipt.QuantityMilli > 99999999999999 || receipt.OccurredAt.IsZero() {
		return fmt.Errorf("invalid prepared receipt")
	}

	quantity := fmt.Sprintf("%d.%03d", receipt.QuantityMilli/1000, receipt.QuantityMilli%1000)
	const query = `
		INSERT INTO stock_movements (
			batch_id, type, delta_quantity, remaining_quantity_after,
			product_name_snapshot, unit_snapshot, actor_id, occurred_at
		)
		VALUES ($1, 'receipt', $2::numeric, $2::numeric, $3, $4, $5, $6)
	`
	_, err := tx.Exec(ctx, query,
		receipt.BatchID.String(), quantity, receipt.ProductName, receipt.Unit,
		receipt.ActorID.String(), receipt.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("insert supply receipt: %w", err)
	}
	return nil
}
