package writeoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func conditions(f model.StockMovementFilter) (string, []any) {
	clauses := []string{}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.ProductID != nil {
		add("b.product_id=$%d", *f.ProductID)
	}
	if f.BatchID != nil {
		add("m.batch_id=$%d", *f.BatchID)
	}
	if f.OrderID != nil {
		add("oi.order_id=$%d", *f.OrderID)
	}
	if f.Type != "" {
		add("m.type=$%d", f.Type)
	}
	if f.DateFrom != nil {
		add("m.occurred_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("m.occurred_at < $%d", *f.DateTo)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) List(
	ctx context.Context,
	q repository.DBTX,
	f model.StockMovementFilter,
) ([]dto.StockMovement, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := conditions(f)
	const from = ` FROM stock_movements m JOIN batches b ON b.id=m.batch_id LEFT JOIN order_items oi ON oi.id=m.order_item_id LEFT JOIN users u ON u.id=m.actor_id`
	var total int64
	if err = tx.QueryRow(ctx, "SELECT count(*)"+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count stock movements: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		fmt.Sprintf(
			`SELECT m.id,m.batch_id,b.product_id,oi.order_id,m.order_item_id,m.type::text,m.delta_quantity::text,m.remaining_quantity_after::text,m.product_name_snapshot,m.unit_snapshot::text,m.actor_id,u.name,m.occurred_at,m.note`+from+where+` ORDER BY m.occurred_at DESC,m.id DESC LIMIT $%d OFFSET $%d`,
			len(args)+1,
			len(args)+2,
		),
		append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list stock movements: %w", err)
	}
	defer rows.Close()
	result := make([]dto.StockMovement, 0)
	for rows.Next() {
		var m dto.StockMovement
		var actorID *uuid.UUID
		var actorName *string
		if err = rows.Scan(
			&m.ID,
			&m.BatchID,
			&m.ProductID,
			&m.OrderID,
			&m.OrderItemID,
			&m.Type,
			&m.DeltaQuantity,
			&m.RemainingQuantityAfter,
			&m.ProductNameSnapshot,
			&m.UnitSnapshot,
			&actorID,
			&actorName,
			&m.OccurredAt,
			&m.Note,
		); err != nil {
			return nil, 0, fmt.Errorf("scan stock movement: %w", err)
		}
		m.OccurredAt = m.OccurredAt.UTC()
		if actorID != nil && actorName != nil {
			m.Actor = &dto.ActorBrief{ID: *actorID, Name: *actorName}
		}
		result = append(result, m)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read stock movements: %w", err)
	}
	return result, total, nil
}
