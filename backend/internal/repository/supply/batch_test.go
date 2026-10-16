package supply

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

type recordingTx struct {
	pgx.Tx
	args []any
}

func (tx *recordingTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.args = args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestInsertBatchPreservesExpirySnapshot(t *testing.T) {
	produced := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	hours := 72
	expires := produced.Add(72 * time.Hour)
	tx := &recordingTx{}
	err := NewRepository().InsertBatch(context.Background(), tx, &model.NewSupplyBatch{
		ID:            uuid.New(),
		SupplyID:      uuid.New(),
		ProductID:     uuid.New(),
		LineNumber:    1,
		QuantityMilli: 2500,
		Expiry: model.BatchExpiry{
			ProducedAt:     &produced,
			ShelfLifeHours: &hours,
			Source:         model.ExpiryCalculated,
			ExpiresAt:      expires,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.args) != 9 || tx.args[4] != "2.500" || *tx.args[6].(*int) != 72 || tx.args[7] != "calculated" ||
		!tx.args[8].(time.Time).Equal(expires) {
		t.Fatalf("unexpected stored arguments: %+v", tx.args)
	}
}

func TestInsertBatchRequiresTransaction(t *testing.T) {
	if err := NewRepository().InsertBatch(context.Background(), nil, &model.NewSupplyBatch{}); err == nil {
		t.Fatal("expected explicit transaction requirement")
	}
}
