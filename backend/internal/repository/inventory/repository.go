package inventory

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

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) ExpiringThresholdHours(ctx context.Context, q repository.DBTX) (int, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return 0, err
	}
	var hours int
	err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT expiring_threshold_hours FROM restaurant_settings WHERE id=1),24)`).
		Scan(&hours)
	if err != nil {
		return 0, fmt.Errorf("read inventory threshold: %w", err)
	}
	return hours, nil
}

func (r *Repository) List(
	ctx context.Context,
	q repository.DBTX,
	filter model.InventoryFilter,
	now, warningAt time.Time,
) (*model.InventoryResult, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, err
	}
	condition, err := inventoryCondition(filter.Filter)
	if err != nil {
		return nil, err
	}
	order, err := inventoryOrder(filter.Sort)
	if err != nil {
		return nil, err
	}
	args := []any{now.UTC(), warningAt.UTC()}
	conditions := make([]string, 0, 2)
	if filter.Search != nil && *filter.Search != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*filter.Search)
		args = append(args, "%"+literal+"%")
		conditions = append(conditions, fmt.Sprintf(`p.name ILIKE $%d ESCAPE E'\\'`, len(args)))
	}
	if filter.CategoryID != nil {
		args = append(args, filter.CategoryID.String())
		conditions = append(conditions, fmt.Sprintf("p.category_id=$%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	selected := `SELECT p.id,p.name,p.unit::text,p.display_unit::text,p.is_active,p.target_stock,
		c.id AS category_id,c.name AS category_name
		FROM products p JOIN product_categories c ON c.id=p.category_id` + where
	cte := `WITH selected AS (` + selected + `), totals AS (
		SELECT p.id,
			COALESCE(SUM(b.remaining_quantity),0.000) AS physical_quantity,
			COALESCE(SUM(b.remaining_quantity) FILTER (WHERE b.expires_at>$1),0.000) AS available_quantity,
			COALESCE(SUM(b.remaining_quantity) FILTER (WHERE b.expires_at>$1 AND b.expires_at<=$2),0.000) AS expiring_quantity,
			COALESCE(SUM(b.remaining_quantity) FILTER (WHERE b.expires_at<=$1),0.000) AS expired_quantity,
			MIN(b.expires_at) FILTER (WHERE b.expires_at>$1) AS nearest_expires_at,
			COUNT(b.id) AS batches_count
		FROM selected p LEFT JOIN batches b ON b.product_id=p.id AND b.remaining_quantity>0
		GROUP BY p.id
	), aggregated AS (
		SELECT p.*,t.physical_quantity,t.available_quantity,t.expiring_quantity,t.expired_quantity,
			t.nearest_expires_at,t.batches_count,
			CASE WHEN p.target_stock IS NULL THEN NULL ELSE GREATEST(p.target_stock-t.available_quantity,0.000) END AS deficit_quantity
		FROM selected p JOIN totals t ON t.id=p.id
	) `
	result := &model.InventoryResult{
		Items:            make([]model.InventoryItem, 0),
		AttentionBatches: make([]model.InventoryAttentionBatch, 0),
	}
	if err := tx.QueryRow(ctx, cte+`SELECT COUNT(*),COUNT(*) FILTER (WHERE expiring_quantity>0),
		COUNT(*) FILTER (WHERE expired_quantity>0),COUNT(*) FILTER (WHERE available_quantity=0),
		COUNT(*) FILTER (WHERE deficit_quantity>0) FROM aggregated`, args...).Scan(
		&result.Counts.All, &result.Counts.Expiring, &result.Counts.Expired, &result.Counts.OutOfStock, &result.Counts.Deficit); err != nil {
		return nil, fmt.Errorf("count inventory: %w", err)
	}
	switch filter.Filter {
	case "expiring":
		result.Total = result.Counts.Expiring
	case "expired":
		result.Total = result.Counts.Expired
	case "out_of_stock":
		result.Total = result.Counts.OutOfStock
	case "deficit":
		result.Total = result.Counts.Deficit
	default:
		result.Total = result.Counts.All
	}
	if result.Total > 0 {
		query := fmt.Sprintf(`SELECT id,name,category_id,category_name,unit,display_unit,is_active,
			physical_quantity::text,available_quantity::text,expiring_quantity::text,expired_quantity::text,
			target_stock::text,deficit_quantity::text,batches_count,nearest_expires_at
			FROM aggregated %s ORDER BY %s LIMIT $%d OFFSET $%d`, condition, order, len(args)+1, len(args)+2)
		pageArgs := append(append([]any(nil), args...), filter.PageSize, offset)
		rows, err := tx.Query(ctx, cte+query, pageArgs...)
		if err != nil {
			return nil, fmt.Errorf("list inventory: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item model.InventoryItem
			p := &item.Product
			if err := rows.Scan(&p.ID, &p.Name, &p.Category.ID, &p.Category.Name, &p.Unit, &p.DisplayUnit, &p.IsActive,
				&item.PhysicalQuantity, &item.AvailableQuantity, &item.ExpiringQuantity, &item.ExpiredQuantity,
				&item.TargetStock, &item.DeficitQuantity, &item.BatchesCount, &item.NearestExpiresAt); err != nil {
				return nil, fmt.Errorf("scan inventory: %w", err)
			}
			result.Items = append(result.Items, item)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("read inventory: %w", err)
		}
		rows.Close()
	}
	attentionQuery := `WITH selected AS (` + selected + `) SELECT b.id,p.id,p.name,s.id,s.number,b.line_number,
		b.remaining_quantity::text,p.unit,b.expires_at,COUNT(*) OVER()
		FROM selected p JOIN batches b ON b.product_id=p.id JOIN supplies s ON s.id=b.supply_id
		WHERE b.remaining_quantity>0 AND (b.expires_at<=$1 OR (b.expires_at>$1 AND b.expires_at<=$2))
		ORDER BY b.expires_at ASC,b.id ASC LIMIT 20`
	rows, err := tx.Query(ctx, attentionQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list inventory attention: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var batch model.InventoryAttentionBatch
		if err := rows.Scan(
			&batch.BatchID,
			&batch.ProductID,
			&batch.ProductName,
			&batch.SupplyID,
			&batch.SupplyNumber,
			&batch.LineNumber,
			&batch.RemainingQuantity,
			&batch.Unit,
			&batch.ExpiresAt,
			&result.AttentionTotal,
		); err != nil {
			return nil, fmt.Errorf("scan inventory attention: %w", err)
		}
		result.AttentionBatches = append(result.AttentionBatches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read inventory attention: %w", err)
	}
	return result, nil
}

func inventoryCondition(filter string) (string, error) {
	switch filter {
	case "", "all":
		return "", nil
	case "expiring":
		return "WHERE expiring_quantity>0", nil
	case "expired":
		return "WHERE expired_quantity>0", nil
	case "out_of_stock":
		return "WHERE available_quantity=0", nil
	case "deficit":
		return "WHERE deficit_quantity>0", nil
	default:
		return "", &validations.FieldError{Field: "filter", Message: "Недопустимое значение фильтра"}
	}
}

func inventoryOrder(sort string) (string, error) {
	switch sort {
	case "", "name":
		return "name ASC,id ASC", nil
	case "nearest_expires_at":
		return "nearest_expires_at ASC NULLS LAST,id ASC", nil
	default:
		return "", &validations.FieldError{Field: "sort", Message: "Недопустимое значение сортировки"}
	}
}
