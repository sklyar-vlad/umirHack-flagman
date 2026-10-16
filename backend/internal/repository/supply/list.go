package supply

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (r *Repository) List(
	ctx context.Context,
	q repository.DBTX,
	filter model.SupplyFilter,
) ([]model.Supply, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
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
		FROM supplies s JOIN users u ON u.id = s.created_by
		LEFT JOIN batches b ON b.supply_id = s.id
		%s GROUP BY s.id, u.id, u.name
		ORDER BY s.created_at DESC, s.id DESC LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	supplies := make([]model.Supply, 0)
	var total int64
	if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count supplies: %w", err)
	}
	if total == 0 {
		return supplies, total, nil
	}
	pageArgs := append(append([]any(nil), args...), filter.PageSize, offset)
	rows, err := tx.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list supplies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var supply model.Supply
		if err := rows.Scan(&supply.ID, &supply.Number, &supply.CreatedAt, &supply.ReceivedAt,
			&supply.CreatedBy.ID, &supply.CreatedBy.Name, &supply.ItemsCount); err != nil {
			return nil, 0, fmt.Errorf("scan supply: %w", err)
		}
		supply.Status = model.SupplyStatusReceived
		supplies = append(supplies, supply)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read supplies: %w", err)
	}
	return supplies, total, nil
}

func supplyDateBoundary(value time.Time) time.Time {
	boundary := value.Truncate(time.Microsecond)
	if !boundary.Equal(value) {
		boundary = boundary.Add(time.Microsecond)
	}
	return boundary
}
