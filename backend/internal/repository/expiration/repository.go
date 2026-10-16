package expiration

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	writeoffrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
)

type Repository struct {
	batches *writeoffrepository.Repository
}

func NewRepository() *Repository { return &Repository{batches: writeoffrepository.NewRepository()} }

func (r *Repository) Candidates(ctx context.Context, q repository.DBTX) ([]uuid.UUID, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(
		ctx,
		`SELECT id FROM batches WHERE remaining_quantity>0 AND expires_at<=clock_timestamp() ORDER BY product_id,id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list expiration candidates: %w", err)
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) LockExpired(ctx context.Context, q repository.DBTX, id uuid.UUID) (*dto.StockMovement, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	b, err := r.batches.LockBatch(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT auto_writeoff_expired FROM restaurant_settings WHERE id=1 FOR SHARE`).
		Scan(&enabled); err != nil {
		return nil, fmt.Errorf("recheck expiration settings: %w", err)
	}
	if !enabled {
		return nil, nil
	}
	m := &dto.StockMovement{
		ID:                     uuid.New(),
		BatchID:                id,
		ProductID:              b.ProductID,
		Type:                   "expiration",
		DeltaQuantity:          "-" + b.RemainingQuantity,
		RemainingQuantityAfter: "0.000",
		ProductNameSnapshot:    b.ProductName,
		UnitSnapshot:           b.Unit,
	}
	var expired bool
	if err := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() AND remaining_quantity>0,clock_timestamp() FROM batches WHERE id=$1`, id).
		Scan(&expired, &m.OccurredAt); err != nil {
		return nil, fmt.Errorf("recheck expiration balance: %w", err)
	}
	if !expired {
		return nil, nil
	}
	m.OccurredAt = m.OccurredAt.UTC()
	return m, nil
}

func (r *Repository) WriteExpiration(ctx context.Context, q repository.DBTX, m *dto.StockMovement) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(
		ctx,
		`UPDATE batches SET remaining_quantity=0 WHERE id=$1 AND remaining_quantity=-$2::numeric AND remaining_quantity>0 AND expires_at<=clock_timestamp()`,
		m.BatchID,
		m.DeltaQuantity,
	)
	if err != nil {
		return fmt.Errorf("update expired balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("locked expiration balance changed")
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO stock_movements(id,batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at) VALUES($1,$2,'expiration',$3::numeric,0,$4,$5,NULL,$6)`,
		m.ID,
		m.BatchID,
		m.DeltaQuantity,
		m.ProductNameSnapshot,
		m.UnitSnapshot,
		m.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("insert expiration movement: %w", err)
	}
	return nil
}
