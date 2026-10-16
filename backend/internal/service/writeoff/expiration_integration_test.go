package writeoff_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	dashboardrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	expirationrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/expiration"
	orderrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	settingsrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/settings"
	dashboardservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dashboard"
	expirationservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/expiration"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	settingsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/settings"
	"go.uber.org/zap"
)

func (f *fixture) expiry() *expirationservice.Service {
	base := repository.NewRepository(f.pool, zap.NewNop())
	return expirationservice.NewService(expirationrepo.NewRepository(), base, base, zap.NewNop())
}

func (f *fixture) enable(t *testing.T, enabled bool) {
	t.Helper()
	base := repository.NewRepository(f.pool, zap.NewNop())
	settings := settingsservice.NewService(settingsrepo.NewRepository(), base, base)
	before, err := settings.Get(f.actor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(
		f.actor(),
		dto.UpdateSettingsRequest{AutoWriteOffExpired: &enabled, ExpectedRevision: before.Revision},
	); err != nil {
		t.Fatal(err)
	}

	f.exec(t, `DELETE FROM audit_log WHERE entity_type='settings'`)
	f.exec(t, `DELETE FROM outbox_events WHERE event_type='settings.changed'`)
}

func (f *fixture) expirationCounts(t *testing.T) [3]int {
	t.Helper()
	var c [3]int
	err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM stock_movements WHERE type='expiration'),(SELECT count(*) FROM audit_log WHERE action='expiration'),(SELECT count(*) FROM outbox_events WHERE event_type='inventory.changed')`).
		Scan(&c[0], &c[1], &c[2])
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestExpirationDisabledVisibleButUnavailableToOrders(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{} {
		t.Fatal("disabled expiry mutated stock")
	}
	base := repository.NewRepository(f.pool, zap.NewNop())
	dashboard, err := dashboardservice.NewService(dashboardrepo.NewRepository(), base).Get(f.actor(), "admin")
	if err != nil || dashboard.ExpiredBatchesCount != 1 || dashboard.AttentionTotal != 1 {
		t.Fatal("disabled expiration disappeared from dashboard", dashboard, err)
	}
	orders := orderservice.NewCreateService(orderrepo.NewRepository(), base, idempotency.NewService(base))
	result, err := orders.CreateIdempotent(f.actor(), f.user, uuid.New(), f.orderBody(t, "1"))
	if err != nil || result.Status != 409 {
		t.Fatal("expired stock available to order", result, err)
	}
	if f.balance(t) != "10.000" {
		t.Fatal("order consumed expired stock")
	}
}

func TestExpirationCandidatesAndExactBalances(t *testing.T) {
	for _, unit := range []string{"ml", "pcs"} {
		t.Run(unit, func(t *testing.T) {
			f := newFixture(t)
			f.enable(t, true)
			if unit == "pcs" {
				f.exec(t, `UPDATE products SET unit='pcs',display_unit='pcs' WHERE id=$1`, f.product)
			}
			f.exec(t, `UPDATE batches SET expires_at=clock_timestamp() WHERE id=$1`, f.batch)
			empty, valid := uuid.New(), uuid.New()
			f.exec(
				t,
				`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$3,$4,2,1,0,'explicit',clock_timestamp()-interval '1 day'),($2,$3,$4,3,2,2,'explicit',clock_timestamp()+interval '1 day')`,
				empty,
				valid,
				f.product,
				f.supply,
			)
			service := f.expiry()
			if err := service.Expire(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := service.Expire(f.ctx); err != nil {
				t.Fatal(err)
			}
			f.reconcile(t, "0.000")
			if f.expirationCounts(t) != [3]int{1, 1, 1} {
				t.Fatal("duplicate or skipped movement", f.expirationCounts(t))
			}
			var total string
			if err := f.pool.QueryRow(f.ctx, `SELECT sum(remaining_quantity)::text FROM batches WHERE id IN ($1,$2)`, empty, valid).
				Scan(&total); err != nil ||
				total != "2.000" {
				t.Fatal("valid/zero batch touched", total, err)
			}
		})
	}
}

func TestExpirationFreshBalanceExpiryAndSettingsAfterWait(t *testing.T) {
	for _, change := range []string{"balance", "expiry", "disabled", "product"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			f.enable(t, true)
			f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			sql := `SELECT id FROM batches WHERE id=$1 FOR UPDATE`
			id := f.batch
			if change == "product" {
				sql = `SELECT id FROM products WHERE id=$1 FOR UPDATE`
				id = f.product
			}
			if _, err := tx.Exec(f.ctx, sql, id); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- f.expiry().Expire(f.ctx) }()
			query := "%FROM batches%FOR UPDATE OF b%"
			if change == "product" {
				query = "%FROM products%FOR UPDATE%"
			}
			waitLock(t, f, query)
			switch change {
			case "balance":
				if _, err := tx.Exec(
					f.ctx,
					`UPDATE batches SET remaining_quantity=7 WHERE id=$1`,
					f.batch,
				); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(
					f.ctx,
					`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note) VALUES($1,'manual_writeoff',-3,7,'Молоко','ml',$2,clock_timestamp(),'Test partial write-off')`,
					f.batch,
					f.user,
				); err != nil {
					t.Fatal(err)
				}
			case "expiry":
				if _, err := tx.Exec(
					f.ctx,
					`UPDATE batches SET expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`,
					f.batch,
				); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				f.enable(t, false)
			case "product":
				if _, err := tx.Exec(
					f.ctx,
					`UPDATE products SET name='Fresh name' WHERE id=$1`,
					f.product,
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if change == "expiry" || change == "disabled" {
				if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{} {
					t.Fatal("stale expiration check")
				}
				return
			}
			f.reconcile(t, "0.000")
			var delta, name string
			if err := f.pool.QueryRow(f.ctx, `SELECT delta_quantity::text,product_name_snapshot FROM stock_movements WHERE type='expiration'`).
				Scan(&delta, &name); err != nil {
				t.Fatal(err)
			}
			if change == "balance" && delta != "-7.000" {
				t.Fatal("stale balance", delta)
			}
			if change == "product" && name != "Fresh name" {
				t.Fatal("stale product", name)
			}
		})
	}
}

func TestTimedExpirationWithinMinuteAndShutdown(t *testing.T) {
	f := newFixture(t)

	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()+interval '500 milliseconds' WHERE id=$1`, f.batch)
	base := repository.NewRepository(f.pool, zap.NewNop())
	worker := expirationservice.NewWorker(settingsrepo.NewRepository(), base, f.expiry(), zap.NewNop())
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	defer func() { cancel(); <-done }()
	time.Sleep(time.Second)
	if f.balance(t) != "10.000" {
		t.Fatal("disabled worker wrote stock")
	}
	f.enable(t, true)
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) && f.balance(t) != "0.000" {
		time.Sleep(50 * time.Millisecond)
	}
	if f.balance(t) != "0.000" || f.expirationCounts(t) != [3]int{1, 1, 1} {
		t.Fatal("timed pass missed expiry")
	}
}
