package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (r *Repository) ListOrders(ctx context.Context, q repository.DBTX, filter model.OrderHistoryFilter) ([]model.OrderHistory, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := historyConditions(filter, `(o.number::text ILIKE $1 ESCAPE E'\\' OR EXISTS (
		SELECT 1 FROM order_items oi WHERE oi.order_id=o.id AND oi.dish_name_snapshot ILIKE $1 ESCAPE E'\\'))`)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders o `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}
	result := make([]model.OrderHistory, 0)
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT o.id,o.number,o.created_at,u.id,u.name
		FROM orders o JOIN users u ON u.id=o.created_by %s
		ORDER BY o.created_at DESC,o.id DESC LIMIT $%d OFFSET $%d`, where, len(args)+1, len(args)+2),
		append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var order model.OrderHistory
		if err := rows.Scan(&order.ID, &order.Number, &order.CreatedAt, &order.CreatedBy.ID, &order.CreatedBy.Name); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scan order: %w", err)
		}
		result = append(result, order)
		ids = append(ids, order.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read orders: %w", err)
	}
	if len(ids) == 0 {
		return result, total, nil
	}
	items, err := r.historyItems(ctx, tx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range result {
		result[i].Items = items[result[i].ID]
	}
	return result, total, nil
}

// historyConditions only accepts SQL expressions from this package. User input
// is always bound, with LIKE metacharacters escaped for literal substring search.
func historyConditions(filter model.OrderHistoryFilter, searchSQL string) (string, []any) {
	conditions := make([]string, 0)
	args := make([]any, 0)
	if filter.Search != "" {
		conditions = append(conditions, searchSQL)
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter.Search)
		args = append(args, "%"+literal+"%")
	}
	if filter.DateFrom != nil {
		args = append(args, historyDateBoundary(*filter.DateFrom))
		conditions = append(conditions, fmt.Sprintf("o.created_at >= $%d", len(args)))
	}
	if filter.DateTo != nil {
		args = append(args, historyDateBoundary(*filter.DateTo))
		conditions = append(conditions, fmt.Sprintf("o.created_at < $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

// PostgreSQL timestamps have microsecond precision. Ceiling preserves both
// comparisons when an RFC3339Nano boundary lies between stored timestamps.
func historyDateBoundary(value time.Time) time.Time {
	truncated := value.UTC().Truncate(time.Microsecond)
	if truncated.Before(value) {
		return truncated.Add(time.Microsecond)
	}
	return truncated
}

func (r *Repository) historyItems(ctx context.Context, q repository.DBTX, ids []uuid.UUID) (map[uuid.UUID][]model.OrderHistoryItem, error) {
	rows, err := q.Query(ctx, `SELECT oi.id,oi.order_id,oi.dish_id,oi.recipe_version_id,
		oi.dish_name_snapshot,oi.portions,rv.version
		FROM order_items oi JOIN recipe_versions rv ON rv.id=oi.recipe_version_id AND rv.dish_id=oi.dish_id
		WHERE oi.order_id=ANY($1::uuid[]) ORDER BY oi.order_id,oi.dish_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("read historical order items: %w", err)
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]model.OrderHistoryItem, len(ids))
	for _, id := range ids {
		result[id] = make([]model.OrderHistoryItem, 0)
	}
	for rows.Next() {
		var item model.OrderHistoryItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.DishID, &item.RecipeVersionID,
			&item.DishName, &item.Portions, &item.RecipeVersion); err != nil {
			return nil, fmt.Errorf("scan historical order item: %w", err)
		}
		result[item.OrderID] = append(result[item.OrderID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate historical order items: %w", err)
	}
	return result, nil
}
