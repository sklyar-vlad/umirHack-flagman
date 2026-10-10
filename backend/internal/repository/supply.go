package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (r *repository) ListSupplies(ctx context.Context, filter model.SupplyFilter) ([]model.Supply, int64, error) {
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, 0, err
	}
	var conditions []string
	var args []any
	argIdx := 1
	if filter.Search != nil && *filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(`s.number ILIKE $%d ESCAPE E'\\'`, argIdx))
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*filter.Search)
		args = append(args, "%"+literal+"%")
		argIdx++
	}
	if filter.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("s.created_at >= $%d", argIdx))
		args = append(args, supplyDateBoundary(*filter.DateFrom))
		argIdx++
	}
	if filter.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("s.created_at < $%d", argIdx))
		args = append(args, supplyDateBoundary(*filter.DateTo))
		argIdx++
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM supplies s %s`, whereClause)
	query := fmt.Sprintf(`
        SELECT s.id, s.number, s.created_at, s.received_at, u.id, u.name, COUNT(b.id)
        FROM supplies s
        JOIN users u ON u.id = s.created_by
        LEFT JOIN batches b ON b.supply_id = s.id
        %s
        GROUP BY s.id, u.id, u.name
        ORDER BY s.created_at DESC, s.id DESC
        LIMIT $%d OFFSET $%d
    `, whereClause, argIdx, argIdx+1)
	supplies := make([]model.Supply, 0)
	var total int64
	// Count and page must see the same data if a receipt commits between reads.
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err = r.WithinTx(ctx, options, func(tx DBTX) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count supplies: %w", err)
		}
		if total == 0 {
			return nil
		}
		pageArgs := append(append([]any(nil), args...), filter.PageSize, offset)
		rows, err := tx.Query(ctx, query, pageArgs...)
		if err != nil {
			return fmt.Errorf("list supplies: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var supply model.Supply
			if err := rows.Scan(&supply.ID, &supply.Number, &supply.CreatedAt, &supply.ReceivedAt,
				&supply.CreatedBy.ID, &supply.CreatedBy.Name, &supply.ItemsCount); err != nil {
				return fmt.Errorf("scan supply: %w", err)
			}
			supply.Status = model.SupplyStatusReceived
			supplies = append(supplies, supply)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read supplies: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return supplies, total, nil
}

// PostgreSQL stores microseconds. Rounding bounds upward preserves both >= from
// and < to when the client supplies fractional seconds finer than this grid.
func supplyDateBoundary(value time.Time) time.Time {
	boundary := value.Truncate(time.Microsecond)
	if !boundary.Equal(value) {
		boundary = boundary.Add(time.Microsecond)
	}
	return boundary
}

func (r *repository) GetSupplyByID(ctx context.Context, id uuid.UUID) (*model.SupplyDetails, error) {
	const supplyQuery = `
        SELECT s.id, s.number, s.created_at, s.received_at, u.id, u.name,
            COALESCE((SELECT expiring_threshold_hours FROM restaurant_settings WHERE id = 1), 24)
        FROM supplies s
        JOIN users u ON u.id = s.created_by
        WHERE s.id = $1
    `
	const itemsQuery = `
        SELECT b.id, p.id, p.name, p.unit, p.display_unit, b.line_number,
            b.initial_quantity::text, b.remaining_quantity::text, b.produced_at,
            b.shelf_life_hours, b.expiry_source, b.expires_at, b.created_at
        FROM batches b JOIN products p ON p.id = b.product_id
        WHERE b.supply_id = $1
        ORDER BY b.line_number ASC, b.id ASC
    `
	details := &model.SupplyDetails{Items: make([]model.BatchItem, 0)}
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := r.WithinTx(ctx, options, func(tx DBTX) error {
		err := tx.QueryRow(ctx, supplyQuery, id.String()).Scan(
			&details.ID, &details.Number, &details.CreatedAt, &details.ReceivedAt,
			&details.CreatedBy.ID, &details.CreatedBy.Name, &details.ExpiringThresholdHours,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read supply details: %w", err)
		}
		rows, err := tx.Query(ctx, itemsQuery, id.String())
		if err != nil {
			return fmt.Errorf("read supply batches: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item model.BatchItem
			if err := rows.Scan(
				&item.BatchID,
				&item.ProductID,
				&item.ProductName,
				&item.Unit,
				&item.DisplayUnit,
				&item.LineNumber,
				&item.InitialQuantity,
				&item.RemainingQuantity,
				&item.ProducedAt,
				&item.ShelfLifeHours,
				&item.ExpirySource,
				&item.ExpiresAt,
				&item.CreatedAt,
			); err != nil {
				return fmt.Errorf("scan supply batch: %w", err)
			}
			details.Items = append(details.Items, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read supply batch rows: %w", err)
		}
		details.Status = model.SupplyStatusReceived
		details.ItemsCount = len(details.Items)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return details, nil
}
