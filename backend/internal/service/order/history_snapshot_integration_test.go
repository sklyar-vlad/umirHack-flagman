package order

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	"go.uber.org/zap"
)

type historyHookTransactions struct {
	Transactions
	afterCount func()
}

func (h historyHookTransactions) WithinTx(
	ctx context.Context,
	options *pgx.TxOptions,
	fn func(repository.DBTX) error,
) error {
	return h.Transactions.WithinTx(ctx, options, func(q repository.DBTX) error {
		return fn(historyHookTx{Tx: q.(pgx.Tx), afterCount: h.afterCount})
	})
}

type historyHookTx struct {
	pgx.Tx
	afterCount func()
}

func (h historyHookTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := h.Tx.QueryRow(ctx, sql, args...)
	if strings.HasPrefix(sql, "SELECT count(*) FROM ") {
		return historyCountRow{Row: row, afterCount: h.afterCount}
	}
	return row
}

type historyCountRow struct {
	pgx.Row
	afterCount func()
}

func (r historyCountRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	r.afterCount()
	return nil
}

func TestOrderHistoryIntegrationCountAndPageUseOneSnapshot(t *testing.T) {
	for _, attempts := range []bool{false, true} {
		name := "orders"
		if attempts {
			name = "attempts"
		}
		t.Run(name, func(t *testing.T) {
			f := newHistoryFixture(t)
			product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
			f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
			if !attempts {
				f.batch(t, product, "5.000", f.now.Add(time.Hour))
			}
			item := dto.OrderRequestItem{DishID: dish.String(), Portions: 1}
			create := func() uuid.UUID {
				if attempts {
					return createHistoryAttempt(t, f, uuid.New(), item)
				}
				return createHistoryOrder(t, f, item).ID
			}
			first := create()
			hooks := 0
			h := historyHookTransactions{
				Transactions: repository.NewRepository(f.pool, zap.NewNop()),
				afterCount:   func() { hooks++; create() },
			}
			s := NewHistoryService(orderrepository.NewRepository(), h)
			filter := model.OrderHistoryFilter{Page: 1, PageSize: 50}
			if attempts {
				items, total, err := s.ListAttempts(f.ctx, filter)
				if err != nil || total != 1 || len(items) != 1 || items[0].ID != first {
					t.Fatalf("attempt snapshot: %+v %d %v", items, total, err)
				}
			} else {
				items, total, err := s.ListOrders(f.ctx, filter)
				if err != nil || total != 1 || len(items) != 1 || items[0].ID != first {
					t.Fatalf("order snapshot: %+v %d %v", items, total, err)
				}
			}
			if hooks != 1 || f.count(t, map[bool]string{true: "order_attempts", false: "orders"}[attempts]) != 2 {
				t.Fatal("concurrent transaction did not commit between count and page")
			}
		})
	}
}
