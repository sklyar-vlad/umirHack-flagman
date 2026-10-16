package dashboard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type countedTx struct {
	pgx.Tx
	queries *[]string
}

func (tx countedTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	*tx.queries = append(*tx.queries, sql)
	return tx.Tx.Query(ctx, sql, args...)
}

func (tx countedTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	*tx.queries = append(*tx.queries, sql)
	return tx.Tx.QueryRow(ctx, sql, args...)
}

type countedTransactions struct {
	Transactions
	t       *testing.T
	queries []string
}

func (s *countedTransactions) WithinTx(ctx context.Context, opts *pgx.TxOptions, fn func(repository.DBTX) error) error {
	if opts.IsoLevel != pgx.RepeatableRead || opts.AccessMode != pgx.ReadOnly {
		s.t.Fatal("dashboard needs read-only repeatable-read")
	}
	return s.Transactions.WithinTx(ctx, opts, func(q repository.DBTX) error {
		return fn(countedTx{Tx: q.(pgx.Tx), queries: &s.queries})
	})
}

func TestDashboardQueryCountIndependentOfDishCountIntegration(t *testing.T) {
	f := newDashboardFixture(t)
	p := f.product(t, "Ingredient")
	f.batch(t, p, "1000.000", f.now.AddDate(0, 0, 1))
	for _, count := range []int{1, 100} {
		for i := range count {
			f.dish(t, fmt.Sprintf("Dish %d/%d", count, i), true, testIngredient{p, "1.000"})
		}
		for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
			counter := &countedTransactions{Transactions: f.service.transactions, t: t}

			base := f.service.transactions
			f.service.transactions = counter
			f.get(t, role)
			f.service.transactions = base
			if len(counter.queries) != 4 {
				t.Fatalf("%d dishes, %s: %d reads", count, role, len(counter.queries))
			}
			aggregates := 0
			for _, query := range counter.queries {
				if strings.Contains(query, "SUM(remaining_quantity)") {
					aggregates++
				}
				if strings.Contains(query, "FROM dishes") && strings.Contains(query, "FROM batches") {
					t.Fatal("recipe query rereads stock")
				}
			}
			if aggregates != 1 {
				t.Fatalf("stock aggregated %d times", aggregates)
			}
		}
	}
}
