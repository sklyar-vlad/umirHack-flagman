package order

import (
	"context"
	"fmt"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (r *Repository) ListAttempts(
	ctx context.Context,
	q repository.DBTX,
	filter model.OrderHistoryFilter,
) ([]model.OrderAttemptHistory, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := historyConditions(filter, `(o.id::text ILIKE $1 ESCAPE E'\\' OR EXISTS (
		SELECT 1 FROM jsonb_array_elements(o.items_snapshot) item WHERE item->>'dish_name' ILIKE $1 ESCAPE E'\\'))`)
	if filter.ReasonCode != "" {
		if where == "" {
			where = "WHERE "
		} else {
			where += " AND "
		}
		args = append(args, filter.ReasonCode)
		where += fmt.Sprintf("o.reason_code=$%d", len(args))
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM order_attempts o `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count order attempts: %w", err)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT o.id,o.created_at,o.reason_code,
		o.items_snapshot,o.shortages,o.issues,u.id,u.name
		FROM order_attempts o JOIN users u ON u.id=o.created_by %s
		ORDER BY o.created_at DESC,o.id DESC LIMIT $%d OFFSET $%d`, where, len(args)+1, len(args)+2),
		append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list order attempts: %w", err)
	}
	defer rows.Close()
	result := make([]model.OrderAttemptHistory, 0)
	for rows.Next() {
		var attempt model.OrderAttemptHistory
		if err := rows.Scan(&attempt.ID, &attempt.CreatedAt, &attempt.ReasonCode, &attempt.ItemsSnapshot,
			&attempt.Shortages, &attempt.Issues, &attempt.ActorID, &attempt.ActorName); err != nil {
			return nil, 0, fmt.Errorf("scan order attempt: %w", err)
		}
		result = append(result, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate order attempts: %w", err)
	}
	return result, total, nil
}
