package order

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
)

func waitOrderBlocked(t *testing.T, f *orderFixture, pid uint32) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		if err := f.pool.QueryRow(f.ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, int32(pid)).
			Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("order never waited for row lock")
		case <-tick.C:
		}
	}
}

func TestOrderStockIntegrationFreshTimeAfterLocks(t *testing.T) {
	for _, table := range []string{"products", "batches"} {
		t.Run(table, func(t *testing.T) {
			f := newOrderFixture(t)
			product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
			f.batch(t, product, "1.000", f.now.Add(time.Hour))
			f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
			holder, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback(f.ctx)
			if _, err := holder.Exec(f.ctx, fmt.Sprintf("SELECT id FROM %s FOR UPDATE", table)); err != nil {
				t.Fatal(err)
			}
			conn, err := f.pool.Acquire(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			tx, err := conn.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			items, err := normalizeBasket(
				dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 1}}},
			)
			if err != nil {
				t.Fatal(err)
			}
			clock := make(chan time.Time, 1)
			result := make(chan lockedStock, 1)
			errors := make(chan error, 1)
			go func() {
				state, err := lockBasket(
					f.ctx,
					orderrepository.NewRepository(),
					tx,
					items,
					func() time.Time { return <-clock },
				)
				result <- state
				errors <- err
			}()
			waitOrderBlocked(t, f, conn.Conn().PgConn().PID())
			fresh := f.now.Add(2 * time.Hour)
			clock <- fresh
			if err := holder.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			state := <-result
			if err := <-errors; err != nil {
				t.Fatal(err)
			}
			check, err := checkBasket(items, state.dishes)
			if err != nil {
				t.Fatal(err)
			}
			if state.serverTime != fresh || check.CanCreate || len(check.Shortages) != 1 ||
				check.Shortages[0].ReasonCode != "only_expired_stock" {
				t.Fatalf("stale locked stock: %+v %+v", state, check)
			}
		})
	}
}

func TestOrderStockIntegrationRecipeAfterDishWait(t *testing.T) {
	f := newOrderFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.batch(t, product, "10.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(f.ctx)
	if _, err := holder.Exec(f.ctx, `SELECT id FROM dishes WHERE id=$1 FOR UPDATE`, dish); err != nil {
		t.Fatal(err)
	}
	conn, err := f.pool.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	version := 1
	items, _ := normalizeBasket(
		dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 1, RecipeVersion: &version}}},
	)
	result := make(chan lockedStock, 1)
	errors := make(chan error, 1)
	go func() {
		state, err := lockBasket(f.ctx, orderrepository.NewRepository(), tx, items, func() time.Time { return f.now })
		result <- state
		errors <- err
	}()
	waitOrderBlocked(t, f, conn.Conn().PgConn().PID())
	recipe := uuid.New()
	if _, err := holder.Exec(
		f.ctx,
		`INSERT INTO recipe_versions(id,dish_id,version,created_by)VALUES($1,$2,2,$3)`,
		recipe,
		dish,
		f.actor,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(
		f.ctx,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,'2.000')`,
		recipe,
		product,
	); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	state := <-result
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	check, err := checkBasket(items, state.dishes)
	if err != nil {
		t.Fatal(err)
	}
	if check.CanCreate || len(check.Issues) != 1 || check.Issues[0].Code != "recipe_changed" ||
		check.Requirements[0].RequiredQuantity != "2.000" ||
		*state.dishes[dish].RecipeVersionID != recipe {
		t.Fatalf("stale recipe: %+v", check)
	}
}
