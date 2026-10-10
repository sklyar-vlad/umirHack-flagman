package supply

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

func (r *Repository) FindDetails(ctx context.Context, q repository.DBTX, id uuid.UUID) (*model.SupplyDetails, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	var details model.SupplyDetails
	const supplyQuery = `
		SELECT s.id, s.number, s.received_at, s.created_at, s.created_by, u.name,
			COALESCE((SELECT expiring_threshold_hours FROM restaurant_settings WHERE id = 1), 24)
		FROM supplies s JOIN users u ON u.id = s.created_by WHERE s.id = $1
	`
	err = tx.QueryRow(ctx, supplyQuery, id.String()).Scan(&details.ID, &details.Number,
		&details.ReceivedAt, &details.CreatedAt, &details.CreatedBy, &details.ActorName, &details.ExpiringThresholdHours)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read supply details: %w", err)
	}
	const itemsQuery = `
		SELECT b.id, b.product_id, p.name, p.unit::text, p.display_unit::text, b.line_number,
			b.initial_quantity::text, b.remaining_quantity::text, b.produced_at, b.shelf_life_hours,
			b.expiry_source::text, b.expires_at, b.created_at
		FROM batches b JOIN products p ON p.id = b.product_id
		WHERE b.supply_id = $1 ORDER BY b.line_number, b.id
	`
	rows, err := tx.Query(ctx, itemsQuery, id.String())
	if err != nil {
		return nil, fmt.Errorf("read supply items: %w", err)
	}
	defer rows.Close()
	details.Items = make([]model.SupplyItemDetails, 0)
	for rows.Next() {
		var item model.SupplyItemDetails
		if err := rows.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.Unit, &item.DisplayUnit,
			&item.LineNumber, &item.InitialQuantity, &item.RemainingQuantity, &item.Expiry.ProducedAt,
			&item.Expiry.ShelfLifeHours, &item.Expiry.Source, &item.Expiry.ExpiresAt, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan supply item: %w", err)
		}
		details.Items = append(details.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate supply items: %w", err)
	}
	return &details, nil
}
