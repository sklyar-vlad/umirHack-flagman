package batch

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (r *Repository) ProductExists(ctx context.Context, q repository.DBTX, id uuid.UUID) (bool, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, id.String()).
		Scan(&exists); err != nil {
		return false, fmt.Errorf("find batch product: %w", err)
	}
	return exists, nil
}

func (r *Repository) ExpiringThresholdHours(ctx context.Context, q repository.DBTX) (int, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return 0, err
	}
	var hours int
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT expiring_threshold_hours FROM restaurant_settings WHERE id=1),24)`).
		Scan(&hours); err != nil {
		return 0, fmt.Errorf("read batch threshold: %w", err)
	}
	return hours, nil
}

func (r *Repository) List(
	ctx context.Context,
	q repository.DBTX,
	filter model.BatchFilter,
	now, warningAt time.Time,
) ([]model.Batch, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, 0, err
	}
	condition, err := batchCondition(filter.Status)
	if err != nil {
		return nil, 0, err
	}
	order, err := batchOrder(filter.Sort)
	if err != nil {
		return nil, 0, err
	}
	args := []any{filter.ProductID.String(), now.UTC(), warningAt.UTC()}
	supplyCondition := ""
	if filter.SupplyID != nil {
		args = append(args, filter.SupplyID.String())
		supplyCondition = " AND b.supply_id=$4"
	}
	// Depleted takes precedence over expiry, including exactly expires_at = now.
	cte := `WITH selected AS (
		SELECT b.*,p.unit::text AS unit,s.number AS supply_number,s.received_at,
			CASE WHEN b.remaining_quantity=0 THEN 'depleted'
				WHEN b.expires_at<=$2 THEN 'expired'
				WHEN b.expires_at<=$3 THEN 'expiring' ELSE 'valid' END AS status
		FROM batches b JOIN products p ON p.id=b.product_id JOIN supplies s ON s.id=b.supply_id
		WHERE b.product_id=$1` + supplyCondition + `) `
	var total int64
	if err := tx.QueryRow(ctx, cte+"SELECT COUNT(*) FROM selected "+condition, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count batches: %w", err)
	}
	batches := make([]model.Batch, 0)
	if total == 0 {
		return batches, total, nil
	}
	query := fmt.Sprintf(`SELECT id,product_id,unit,supply_id,supply_number,received_at,line_number,
		initial_quantity::text,remaining_quantity::text,produced_at,shelf_life_hours,expiry_source::text,expires_at,created_at
		FROM selected %s ORDER BY %s LIMIT $%d OFFSET $%d`, condition, order, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, offset)
	rows, err := tx.Query(ctx, cte+query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list batches: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b model.Batch
		if err := rows.Scan(&b.ID, &b.ProductID, &b.Unit, &b.SupplyID, &b.SupplyNumber, &b.ReceivedAt, &b.LineNumber,
			&b.InitialQuantity, &b.RemainingQuantity, &b.Expiry.ProducedAt, &b.Expiry.ShelfLifeHours,
			&b.Expiry.Source, &b.Expiry.ExpiresAt, &b.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan batch: %w", err)
		}
		batches = append(batches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read batches: %w", err)
	}
	return batches, total, nil
}

func batchCondition(status string) (string, error) {
	switch status {
	case "":
		return "WHERE remaining_quantity>0", nil
	case "all":
		return "", nil
	case "valid", "expiring", "expired", "depleted":
		return "WHERE status='" + status + "'", nil
	default:
		return "", &validations.FieldError{Field: "status", Message: "Недопустимое значение фильтра"}
	}
}

func batchOrder(sort string) (string, error) {
	switch sort {
	case "", "expires_at":
		return "expires_at ASC,id ASC", nil
	case "-created_at":
		return "created_at DESC,id ASC", nil
	default:
		return "", &validations.FieldError{Field: "sort", Message: "Недопустимое значение сортировки"}
	}
}
