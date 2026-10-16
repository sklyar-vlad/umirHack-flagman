package analytics

import (
	"context"
	"fmt"
	"strings"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// ProductFlow counts committed ledger entries, including historical products.
// Grouping by snapshot unit prevents adding incompatible quantities after edits.
func (r *Repository) ProductFlow(
	ctx context.Context,
	q repository.DBTX,
	f model.AnalyticsFilter,
) ([]dto.ProductFlowAnalytics, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	clauses, args := []string{"true"}, []any{}
	add := func(sql string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}
	if f.ProductID != nil {
		add("b.product_id=$%d", *f.ProductID)
	}
	if f.DateFrom != nil {
		add("m.occurred_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("m.occurred_at < $%d", *f.DateTo)
	}
	cte := `WITH grouped AS (SELECT b.product_id,m.unit_snapshot::text AS unit,
 (array_agg(m.product_name_snapshot ORDER BY m.occurred_at DESC,m.id DESC))[1] AS product_name,
 COALESCE(sum(m.delta_quantity) FILTER (WHERE m.type='receipt'),0.000)::text AS received,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type='order_consumption'),0.000)::text AS ordered,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type='expiration'),0.000)::text AS expired,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type='manual_writeoff'),0.000)::text AS manual,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type IN ('expiration','manual_writeoff')),0.000)::text AS written_off,
 count(*) AS movements FROM stock_movements m JOIN batches b ON b.id=m.batch_id WHERE ` + strings.Join(clauses, " AND ") + ` GROUP BY b.product_id,m.unit_snapshot) `
	var total int64
	if err := tx.QueryRow(ctx, cte+"SELECT count(*) FROM grouped", args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(
		ctx,
		fmt.Sprintf(
			cte+`SELECT product_id,product_name,unit,received,ordered,expired,manual,written_off,movements FROM grouped ORDER BY product_name,product_id,unit LIMIT $%d OFFSET $%d`,
			len(args)+1,
			len(args)+2,
		),
		append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]dto.ProductFlowAnalytics, 0)
	for rows.Next() {
		var item dto.ProductFlowAnalytics
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.Unit,
			&item.ReceivedQuantity,
			&item.OrderedQuantity,
			&item.ExpiredQuantity,
			&item.ManualQuantity,
			&item.WrittenOffQuantity,
			&item.MovementsCount,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func conditions(f model.AnalyticsFilter, losses bool) (string, []any) {
	clauses := []string{}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	timestamp := "a.created_at"
	if losses {
		clauses = append(clauses, "m.type IN ('expiration','manual_writeoff')")
		timestamp = "m.occurred_at"
		if f.ProductID != nil {
			add("b.product_id=$%d", *f.ProductID)
		}
		if f.Reason != "" && f.Reason != "all" {
			add("m.type=$%d", f.Reason)
		}
	} else {
		clauses = append(clauses, "a.reason_code='INSUFFICIENT_STOCK'")
	}
	if f.DateFrom != nil {
		add(timestamp+" >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add(timestamp+" < $%d", *f.DateTo)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) WriteOffs(
	ctx context.Context,
	q repository.DBTX,
	f model.AnalyticsFilter,
) ([]dto.WriteOffAnalytics, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := conditions(f, true)
	// Names are historical labels: a rename must not split a product/unit group.
	cte := `WITH grouped AS (SELECT b.product_id,m.unit_snapshot::text AS unit,
 (array_agg(m.product_name_snapshot ORDER BY m.occurred_at DESC,m.id DESC))[1] AS product_name,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type='expiration'),0.000)::text AS expired,
 COALESCE(sum(-m.delta_quantity) FILTER (WHERE m.type='manual_writeoff'),0.000)::text AS manual,
 sum(-m.delta_quantity)::text AS quantity,count(*) AS movements
 FROM stock_movements m JOIN batches b ON b.id=m.batch_id` + where + ` GROUP BY b.product_id,m.unit_snapshot) `
	var total int64
	if err := tx.QueryRow(ctx, cte+"SELECT count(*) FROM grouped", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count write-off analytics: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		fmt.Sprintf(
			cte+`SELECT product_id,product_name,unit,expired,manual,quantity,movements FROM grouped ORDER BY product_name,product_id,unit LIMIT $%d OFFSET $%d`,
			len(args)+1,
			len(args)+2,
		),
		append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list write-off analytics: %w", err)
	}
	defer rows.Close()
	items := make([]dto.WriteOffAnalytics, 0)
	for rows.Next() {
		var item dto.WriteOffAnalytics
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.Unit,
			&item.ExpiredWrittenOffQuantity,
			&item.ManualWrittenOffQuantity,
			&item.TotalWrittenOffQuantity,
			&item.MovementsCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan write-off analytics: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read write-off analytics: %w", err)
	}
	return items, total, nil
}
