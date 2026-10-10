package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func (r *repository) ListSupplies(ctx context.Context, filter model.SupplyFilter) ([]model.Supply, int64, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Search != nil && *filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("s.number ILIKE $%d", argIdx))
		args = append(args, "%"+*filter.Search+"%")
		argIdx++
	}

	if filter.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("s.received_at >= $%d", argIdx))
		args = append(args, *filter.DateFrom)
		argIdx++
	}

	if filter.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("s.received_at <= $%d", argIdx))
		args = append(args, *filter.DateTo)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM supplies s
		%s
	`, whereClause)

	var total int64
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []model.Supply{}, 0, nil
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT 
			s.id,
			s.number,
			s.created_at,
			s.received_at,
			u.id AS user_id,
			u.name AS user_name,
			COUNT(b.id) AS items_count
		FROM supplies s
		JOIN users u ON u.id = s.created_by
		LEFT JOIN batches b ON b.supply_id = s.id
		%s
		GROUP BY s.id, u.id, u.name
		ORDER BY s.received_at DESC, s.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var supplies []model.Supply
	for rows.Next() {
		var s model.Supply
		err := rows.Scan(
			&s.ID,
			&s.Number,
			&s.CreatedAt,
			&s.ReceivedAt,
			&s.CreatedBy.ID,
			&s.CreatedBy.Name,
			&s.ItemsCount,
		)
		if err != nil {
			return nil, 0, err
		}
		s.Status = model.SupplyStatusReceived
		supplies = append(supplies, s)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return supplies, total, nil
}

func (r *repository) GetSupplyByID(ctx context.Context, id uuid.UUID) (*model.Supply, []model.BatchItem, error) {
	supplyQuery := `
		SELECT 
			s.id,
			s.number,
			s.created_at,
			s.received_at,
			u.id AS user_id,
			u.name AS user_name
		FROM supplies s
		JOIN users u ON u.id = s.created_by
		WHERE s.id = $1
	`

	var s model.Supply
	err := r.pool.QueryRow(ctx, supplyQuery, id).Scan(
		&s.ID,
		&s.Number,
		&s.CreatedAt,
		&s.ReceivedAt,
		&s.CreatedBy.ID,
		&s.CreatedBy.Name,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	s.Status = model.SupplyStatusReceived

	itemsQuery := `
		SELECT 
			b.id AS batch_id,
			p.id AS product_id,
			p.name AS product_name,
			p.unit,
			p.display_unit,
			b.line_number,
			b.initial_quantity::text,
			b.remaining_quantity::text,
			b.produced_at,
			b.shelf_life_hours,
			b.expiry_source,
			b.expires_at,
			b.created_at
		FROM batches b
		JOIN products p ON p.id = b.product_id
		WHERE b.supply_id = $1
		ORDER BY b.line_number ASC, b.id ASC
	`

	rows, err := r.pool.Query(ctx, itemsQuery, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var items []model.BatchItem
	for rows.Next() {
		var bi model.BatchItem
		err := rows.Scan(
			&bi.BatchID,
			&bi.ProductID,
			&bi.ProductName,
			&bi.Unit,
			&bi.DisplayUnit,
			&bi.LineNumber,
			&bi.InitialQuantity,
			&bi.RemainingQuantity,
			&bi.ProducedAt,
			&bi.ShelfLifeHours,
			&bi.ExpirySource,
			&bi.ExpiresAt,
			&bi.CreatedAt,
		)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, bi)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	s.ItemsCount = len(items)
	return &s, items, nil
}