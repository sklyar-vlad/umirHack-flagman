package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (r *Repository) InsertOrder(
	ctx context.Context,
	q repository.DBTX,
	id, actor uuid.UUID,
	now time.Time,
) (model.OrderRecord, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.OrderRecord{}, err
	}
	result := model.OrderRecord{ID: id}
	err = tx.QueryRow(ctx, `WITH created AS (
		INSERT INTO orders(id,created_by,created_at) VALUES($1,$2,$3) RETURNING number,created_at,created_by
	) SELECT c.number,c.created_at,u.id,u.name FROM created c JOIN users u ON u.id=c.created_by`, id, actor, now).
		Scan(&result.Number, &result.CreatedAt, &result.CreatedBy.ID, &result.CreatedBy.Name)
	if err != nil {
		return model.OrderRecord{}, fmt.Errorf("insert order: %w", err)
	}
	return result, nil
}

func (r *Repository) InsertItem(ctx context.Context, q repository.DBTX, item *model.NewOrderItem) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO order_items(id,order_id,dish_id,recipe_version_id,dish_name_snapshot,portions)
		VALUES($1,$2,$3,$4,$5,$6)`, item.ID, item.OrderID, item.DishID, item.RecipeVersionID, item.DishName, item.Portions)
	if err != nil {
		return fmt.Errorf("insert order item: %w", err)
	}
	return nil
}

func (r *Repository) Consume(ctx context.Context, q repository.DBTX, movement *model.OrderConsumption) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	// Verify the planner's running balance against the locked row. Both the
	// guarded update and movement insert are inside the outer transaction.
	tag, err := tx.Exec(ctx, `UPDATE batches SET remaining_quantity=remaining_quantity-$2::numeric
		WHERE id=$1 AND remaining_quantity>=$2::numeric
		AND remaining_quantity-$2::numeric=$3::numeric AND expires_at>$4`,
		movement.BatchID, movement.Quantity, movement.RemainingQuantityAfter, movement.OccurredAt)
	if err != nil {
		return fmt.Errorf("consume order batch: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("locked order batch no longer matches FEFO plan")
	}
	_, err = tx.Exec(ctx, `INSERT INTO stock_movements(id,batch_id,order_item_id,type,delta_quantity,
		remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at)
		VALUES($1,$2,$3,'order_consumption',-$4::numeric,$5::numeric,$6,$7,$8,$9)`,
		movement.ID, movement.BatchID, movement.ItemID, movement.Quantity, movement.RemainingQuantityAfter,
		movement.ProductName, movement.Unit, movement.ActorID, movement.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert order consumption: %w", err)
	}
	return nil
}

func (r *Repository) InsertAttempt(ctx context.Context, q repository.DBTX, attempt *model.OrderAttempt) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO order_attempts(id,created_by,created_at,reason_code,items_snapshot,shortages,issues)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		attempt.ID,
		attempt.ActorID,
		attempt.CreatedAt,
		attempt.ReasonCode,
		attempt.ItemsSnapshot,
		attempt.Shortages,
		attempt.Issues,
	)
	if err != nil {
		return fmt.Errorf("insert order attempt: %w", err)
	}
	return nil
}
