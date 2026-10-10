package supply

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) InsertBatch(
	ctx context.Context,
	tx repository.DBTX,
	batch *model.NewSupplyBatch,
) error {
	if _, err := repository.RequireTransaction(tx); err != nil {
		return err
	}

	if batch == nil || batch.ID == uuid.Nil ||
		batch.SupplyID == uuid.Nil ||
		batch.ProductID == uuid.Nil ||
		batch.LineNumber < 1 ||
		batch.QuantityMilli < 1 ||
		batch.QuantityMilli > 99999999999999 ||
		batch.Expiry.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid prepared batch")
	}

	quantity := fmt.Sprintf(
		"%d.%03d",
		batch.QuantityMilli/1000,
		batch.QuantityMilli%1000,
	)

	const query = `
		INSERT INTO batches (
			id, supply_id, product_id, line_number,
			initial_quantity, remaining_quantity,
			produced_at, shelf_life_hours, expiry_source, expires_at
		)
		VALUES (
			$1, $2, $3, $4,
			$5::numeric, $5::numeric,
			$6, $7, $8, $9
		)
	`

	_, err := tx.Exec(
		ctx,
		query,
		batch.ID.String(),
		batch.SupplyID.String(),
		batch.ProductID.String(),
		batch.LineNumber,
		quantity,
		batch.Expiry.ProducedAt,
		batch.Expiry.ShelfLifeHours,
		string(batch.Expiry.Source),
		batch.Expiry.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert supply batch: %w", err)
	}

	return nil
}
