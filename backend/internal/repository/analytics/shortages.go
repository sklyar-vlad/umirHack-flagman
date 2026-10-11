package analytics

import (
	"context"
	"fmt"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (r *Repository) IngredientShortages(ctx context.Context, q repository.DBTX, f model.AnalyticsFilter) ([]dto.IngredientShortageAnalytics, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := conditions(f, false)
	// Read persisted refusal facts, never current recipes, inventory or product units.
	// Counting distinct attempts also keeps a product represented once per refusal.
	cte := `WITH facts AS (
 SELECT a.id,a.created_at,s.product_id,s.product_name,s.unit,s.missing_quantity::numeric AS missing
 FROM order_attempts a CROSS JOIN LATERAL jsonb_to_recordset(a.shortages)
 AS s(product_id uuid,product_name text,unit text,missing_quantity text)` + where + `),
 grouped AS (SELECT product_id,unit,
 (array_agg(product_name ORDER BY created_at DESC,id DESC,product_name DESC))[1] AS product_name,
 count(DISTINCT id) AS attempts,sum(missing)::text AS quantity
 FROM facts WHERE missing>0 GROUP BY product_id,unit) `
	var total int64
	if err = tx.QueryRow(ctx, cte+"SELECT count(*) FROM grouped", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ingredient shortage analytics: %w", err)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(cte+`SELECT product_id,product_name,unit,attempts,quantity FROM grouped ORDER BY attempts DESC,product_id,unit LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2), append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ingredient shortage analytics: %w", err)
	}
	defer rows.Close()
	items := make([]dto.IngredientShortageAnalytics, 0)
	for rows.Next() {
		var item dto.IngredientShortageAnalytics
		if err = rows.Scan(&item.ProductID, &item.ProductName, &item.Unit, &item.RejectedAttemptsCount, &item.TotalMissingQuantity); err != nil {
			return nil, 0, fmt.Errorf("scan ingredient shortage analytics: %w", err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read ingredient shortage analytics: %w", err)
	}
	return items, total, nil
}
