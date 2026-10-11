package writeoff

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
)

type Repository struct{ *orderrepository.Repository }

func NewRepository() *Repository { return &Repository{orderrepository.NewRepository()} }

// Keep current warehouse rights through all inventory lock waits.
func (r *Repository) LockActor(ctx context.Context, q repository.DBTX, id uuid.UUID) (dto.ActorBrief, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return dto.ActorBrief{}, err
	}
	var active bool
	var role string
	actor := dto.ActorBrief{ID: id}
	err = tx.QueryRow(ctx, `SELECT name,is_active,role::text FROM users WHERE id=$1 FOR SHARE`, id).Scan(&actor.Name, &active, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return actor, apperrors.ErrForbidden
	}
	if err != nil {
		return actor, fmt.Errorf("lock write-off actor: %w", err)
	}
	if !active || (role != "admin" && role != "manager" && role != "storekeeper") {
		return actor, apperrors.ErrForbidden
	}
	return actor, nil
}

func (r *Repository) LockBatch(ctx context.Context, q repository.DBTX, id uuid.UUID) (model.WriteOffBatch, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.WriteOffBatch{}, err
	}
	b := model.WriteOffBatch{ID: id}
	// Read only the immutable product identity before following S10 lock ordering.
	err = tx.QueryRow(ctx, `SELECT product_id FROM batches WHERE id=$1`, id).Scan(&b.ProductID)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, apperrors.ErrNotFound
	}
	if err != nil {
		return b, fmt.Errorf("find write-off batch: %w", err)
	}
	if err = r.LockProducts(ctx, tx, []uuid.UUID{b.ProductID}); err != nil {
		return b, err
	}
	err = tx.QueryRow(ctx, `SELECT p.name,p.unit::text,b.remaining_quantity::text FROM batches b JOIN products p ON p.id=b.product_id WHERE b.id=$1 FOR UPDATE OF b`, id).Scan(&b.ProductName, &b.Unit, &b.RemainingQuantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, apperrors.ErrNotFound
	}
	if err != nil {
		return b, fmt.Errorf("lock write-off batch: %w", err)
	}
	return b, nil
}

func (r *Repository) WriteOff(ctx context.Context, q repository.DBTX, m *dto.StockMovement, amount string) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE batches SET remaining_quantity=remaining_quantity-$2::numeric WHERE id=$1 AND remaining_quantity >= $2::numeric AND remaining_quantity-$2::numeric=$3::numeric`, m.BatchID, amount, m.RemainingQuantityAfter)
	if err != nil {
		return fmt.Errorf("update write-off balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("locked write-off balance changed")
	}
	_, err = tx.Exec(ctx, `INSERT INTO stock_movements(id,batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note) VALUES($1,$2,'manual_writeoff',$3::numeric,$4::numeric,$5,$6,$7,$8,$9)`, m.ID, m.BatchID, m.DeltaQuantity, m.RemainingQuantityAfter, m.ProductNameSnapshot, m.UnitSnapshot, m.Actor.ID, m.OccurredAt, m.Note)
	if err != nil {
		return fmt.Errorf("insert manual write-off: %w", err)
	}
	return nil
}
