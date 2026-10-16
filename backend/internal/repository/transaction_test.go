package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRequireTransactionRejectsNonTransactions(t *testing.T) {
	var nilTx pgx.Tx
	var nilPoolTx *pgxpool.Tx
	for _, test := range []struct {
		name string
		q    DBTX
	}{
		{"nil", nil},
		{"nil interface", nilTx},
		{"typed nil", nilPoolTx},
		{"pool", &pgxpool.Pool{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if tx, err := RequireTransaction(test.q); tx != nil || !errors.Is(err, ErrTransactionRequired) {
				t.Fatalf("expected transaction requirement, got %v, %v", tx, err)
			}
		})
	}
}
