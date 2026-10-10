package procurement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) Load(
	ctx context.Context,
	q repository.DBTX,
	filter model.ProcurementFilter,
	now time.Time,
) ([]model.ProcurementStock, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	args := []any{now.UTC()}
	conditions := []string{"p.target_stock IS NOT NULL"}
	if filter.Search != nil && *filter.Search != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*filter.Search)
		args = append(args, "%"+literal+"%")
		conditions = append(conditions, fmt.Sprintf(`p.name ILIKE $%d ESCAPE E'\\'`, len(args)))
	}
	if filter.CategoryID != nil {
		args = append(args, filter.CategoryID.String())
		conditions = append(conditions, fmt.Sprintf("p.category_id=$%d", len(args)))
	}
	rows, err := tx.Query(ctx, `WITH stock AS (
		SELECT product_id,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at>$1),0.000) AS available,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at<=$1),0.000) AS expired
		FROM batches WHERE remaining_quantity>0 GROUP BY product_id
	)
	SELECT p.id,p.name,p.unit::text,p.display_unit::text,p.target_stock::text,
		COALESCE(s.available,0.000)::text,COALESCE(s.expired,0.000)::text
	FROM products p LEFT JOIN stock s ON s.product_id=p.id
	WHERE `+strings.Join(conditions, " AND ")+` ORDER BY p.name,p.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("load procurement stock: %w", err)
	}
	defer rows.Close()
	items := make([]model.ProcurementStock, 0)
	for rows.Next() {
		var item model.ProcurementStock
		p := &item.Product
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Unit,
			&p.DisplayUnit,
			&p.TargetStock,
			&item.AvailableQuantity,
			&item.ExpiredQuantity,
		); err != nil {
			return nil, fmt.Errorf("scan procurement stock: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read procurement stock: %w", err)
	}
	return items, nil
}
