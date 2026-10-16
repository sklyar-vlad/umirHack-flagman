package writeoff_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	expirationrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/expiration"
	settingsrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/settings"
	writeoffrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	expirationservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/expiration"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestExpirationSystemMovementTypedAuditAndOutbox(t *testing.T) {
	f := newFixture(t)
	f.enable(t, true)
	f.exec(
		t,
		`UPDATE batches SET remaining_quantity=9.999,expires_at=clock_timestamp()-interval '1 hour' WHERE id=$1`,
		f.batch,
	)
	f.exec(
		t,
		`UPDATE stock_movements SET delta_quantity=9.999,remaining_quantity_after=9.999 WHERE batch_id=$1`,
		f.batch,
	)
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	var movementID, entity uuid.UUID
	var actor, orderItem *uuid.UUID
	var note, requestID *string
	var typ, delta, balance, name, unit string
	var at time.Time
	if err := f.pool.QueryRow(f.ctx, `SELECT id,type,delta_quantity::text,remaining_quantity_after::text,product_name_snapshot,unit_snapshot,actor_id,order_item_id,note,occurred_at FROM stock_movements WHERE type='expiration'`).
		Scan(&movementID, &typ, &delta, &balance, &name, &unit, &actor, &orderItem, &note, &at); err != nil {
		t.Fatal(err)
	}
	if typ != "expiration" || delta != "-9.999" || balance != "0.000" || name != "Молоко" || unit != "ml" ||
		actor != nil ||
		orderItem != nil ||
		note != nil ||
		at.IsZero() {
		t.Fatal("incorrect system movement")
	}
	var action, entityType string
	var before, after []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT actor_id,entity_id,entity_type,action,request_id,before_data,after_data FROM audit_log WHERE action='expiration'`).
		Scan(&actor, &entity, &entityType, &action, &requestID, &before, &after); err != nil {
		t.Fatal(err)
	}
	var snapshot model.StockMovementAuditSnapshot
	if err := json.Unmarshal(after, &snapshot); err != nil {
		t.Fatal(err)
	}
	if actor != nil || entity != movementID || entityType != "stock_movement" || action != "expiration" ||
		requestID != nil ||
		before != nil ||
		snapshot.BatchID != f.batch ||
		snapshot.Type != typ ||
		snapshot.DeltaQuantity != delta ||
		snapshot.RemainingQuantityAfter != balance ||
		snapshot.ProductNameSnapshot != name ||
		snapshot.UnitSnapshot != unit ||
		snapshot.Note != nil {
		t.Fatalf("incorrect system audit: %s", after)
	}
	var payload []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT actor_id,payload FROM outbox_events WHERE event_type='inventory.changed'`).
		Scan(&actor, &payload); err != nil {
		t.Fatal(err)
	}
	var event model.OutboxPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if actor != nil || len(event.ProductIDs) != 1 || event.ProductIDs[0] != f.product || len(event.BatchIDs) != 1 ||
		event.BatchIDs[0] != f.batch ||
		event.RequestID != "" ||
		event.OccurredAt.IsZero() {
		t.Fatal("incorrect system outbox")
	}
	for _, topic := range []string{"stock-movements", "inventory", "batches", "dashboard", "dishes", "stop-list", "hot-offers", "procurement-recommendations"} {
		if !bytes.Contains(payload, []byte(topic)) {
			t.Fatal("missing topic", topic)
		}
	}
	f.exec(t, `UPDATE products SET name='Renamed' WHERE id=$1`, f.product)
	history, total, err := f.movements.List(
		f.actor(),
		model.StockMovementFilter{Type: "expiration", Page: 1, PageSize: 50},
	)
	if err != nil || total != 1 || history[0].Actor != nil || history[0].ProductNameSnapshot != "Молоко" {
		t.Fatal("historical system movement changed", history, err)
	}
	f.reconcile(t, "0.000")
}

func TestExpirationImmediateAndDeferredRollbackAndRetry(t *testing.T) {
	for _, table := range []string{"batches", "stock_movements", "audit_log", "outbox_events"} {
		for _, deferred := range []bool{false, true} {
			t.Run(table+map[bool]string{false: " immediate", true: " deferred"}[deferred], func(t *testing.T) {
				f := newFixture(t)
				f.enable(t, true)
				f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
				f.exec(
					t,
					`CREATE FUNCTION reject_expiration() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private expiration secret'; END $$`,
				)
				event := "INSERT"
				if table == "batches" {
					event = "UPDATE"
				}
				if deferred {
					f.exec(
						t,
						"CREATE CONSTRAINT TRIGGER reject_expiration AFTER "+event+" ON "+table+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_expiration()",
					)
				} else {
					f.exec(
						t,
						"CREATE TRIGGER reject_expiration BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_expiration()",
					)
				}
				core, logs := observer.New(zap.ErrorLevel)
				base := repository.NewRepository(f.pool, zap.New(core))
				service := expirationservice.NewService(expirationrepo.NewRepository(), base, base, zap.New(core))
				if err := service.Expire(f.ctx); err == nil {
					t.Fatal("failure was swallowed")
				}
				if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{} {
					t.Fatal("partial expiration", f.balance(t), f.expirationCounts(t))
				}
				if logs.Len() != 1 || logs.All()[0].ContextMap()["sqlstate"] != "P0001" ||
					strings.Contains(logs.All()[0].Message, "secret") ||
					len(logs.All()[0].Context) != 1 {
					t.Fatal("unsafe database logging", logs.All())
				}
				f.exec(t, "DROP TRIGGER reject_expiration ON "+table)
				if err := service.Expire(f.ctx); err != nil {
					t.Fatal(err)
				}
				if err := service.Expire(f.ctx); err != nil {
					t.Fatal(err)
				}
				if f.expirationCounts(t) != [3]int{1, 1, 1} {
					t.Fatal("retry duplicated events")
				}
				f.reconcile(t, "0.000")
			})
		}
	}
}

func TestExpirationAndManualWriteOffBothLockOrders(t *testing.T) {
	for _, first := range []string{"expiration", "manual"} {
		t.Run(first, func(t *testing.T) {
			f := newFixture(t)
			f.enable(t, true)
			f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
			base := repository.NewRepository(f.pool, zap.NewNop())
			gate := &gateEvents{base, make(chan struct{}), make(chan struct{})}
			defer func() {
				select {
				case <-gate.release:
				default:
					close(gate.release)
				}
			}()
			offEvents, expiryEvents := writeoffservice.EventWriter(base), expirationservice.EventWriter(base)
			if first == "expiration" {
				expiryEvents = gate
			} else {
				offEvents = gate
			}
			expiry := expirationservice.NewService(expirationrepo.NewRepository(), base, expiryEvents, zap.NewNop())
			off := writeoffservice.NewService(writeoffrepo.NewRepository(), offEvents, idempotency.NewService(base))
			expirationDone := make(chan error, 1)
			manualDone := make(chan operationResult, 1)
			startExpiry := func() { go func() { expirationDone <- expiry.Expire(f.ctx) }() }
			startManual := func() {
				go func() {
					r, err := off.CreateIdempotent(f.actor(), f.user, uuid.New(), f.body("1.001"))
					manualDone <- operationResult{r, err}
				}()
			}
			if first == "expiration" {
				startExpiry()
			} else {
				startManual()
			}
			select {
			case <-gate.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("first write did not reach gate")
			}
			if first == "expiration" {
				startManual()
			} else {
				startExpiry()
			}
			waitLock(t, f, "%FROM products%FOR UPDATE%")
			close(gate.release)
			if err := <-expirationDone; err != nil {
				t.Fatal(err)
			}
			manual := <-manualDone
			want := 409
			if first == "manual" {
				want = 201
			}
			if manual.err != nil || manual.response.Status != want {
				t.Fatal("manual race", manual.err, manual.response.Status)
			}
			var delta string
			if err := f.pool.QueryRow(f.ctx, `SELECT delta_quantity::text FROM stock_movements WHERE type='expiration'`).
				Scan(&delta); err != nil {
				t.Fatal(err)
			}
			wantDelta := "-10.000"
			if first == "manual" {
				wantDelta = "-8.999"
			}
			if delta != wantDelta {
				t.Fatal("expiration did not use current balance", delta)
			}
			if err := expiry.Expire(f.ctx); err != nil {
				t.Fatal(err)
			}
			f.reconcile(t, "0.000")
			counts := f.expirationCounts(t)
			if counts[0] != 1 || counts[1] != 1 {
				t.Fatal("duplicate expiration", counts)
			}
		})
	}
}

func TestConcurrentExpirationPassesDoNotDuplicateMovement(t *testing.T) {
	f := newFixture(t)
	f.enable(t, true)
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errors <- f.expiry().Expire(f.ctx) }()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.expirationCounts(t) != [3]int{1, 1, 1} {
		t.Fatal("concurrent pass duplicated movement")
	}
	f.reconcile(t, "0.000")
}

func TestExpirationContinuesAfterFailedBatch(t *testing.T) {
	f := newFixture(t)
	f.enable(t, true)
	healthy := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	f.exec(
		t,
		`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,2,2,'explicit',clock_timestamp()-interval '1 second')`,
		healthy,
		f.product,
		f.supply,
	)
	f.exec(
		t,
		`CREATE FUNCTION reject_one_expiration() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.batch_id='`+f.batch.String()+`'::uuid THEN RAISE EXCEPTION 'private secret'; END IF; RETURN NEW; END $$`,
	)
	f.exec(
		t,
		`CREATE TRIGGER reject_one_expiration BEFORE INSERT ON stock_movements FOR EACH ROW EXECUTE FUNCTION reject_one_expiration()`,
	)
	if err := f.expiry().Expire(f.ctx); err == nil {
		t.Fatal("lost failed batch error")
	}
	var remaining string
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id=$1`, healthy).
		Scan(&remaining); err != nil ||
		remaining != "0.000" {
		t.Fatal("failed batch prevented independent expiration", err)
	}
	if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{1, 1, 1} {
		t.Fatal("failure was not isolated")
	}
	f.exec(t, `DROP TRIGGER reject_one_expiration ON stock_movements`)
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.expirationCounts(t) != [3]int{2, 2, 2} {
		t.Fatal("retry not applied exactly once")
	}
}

func TestExpirationCancelDuringLockWaitRollsBack(t *testing.T) {
	f := newFixture(t)
	f.enable(t, true)
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err := tx.Exec(f.ctx, `SELECT id FROM products WHERE id=$1 FOR UPDATE`, f.product); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	base := repository.NewRepository(f.pool, zap.NewNop())
	worker := expirationservice.NewWorker(settingsrepo.NewRepository(), base, f.expiry(), zap.NewNop())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	waitLock(t, f, "%FROM products%FOR UPDATE%")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker cancellation blocked on inventory lock")
	}
	if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{} {
		t.Fatal("canceled worker mutated stock")
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.expiry().Expire(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.reconcile(t, "0.000")
}

func TestExpirationRechecksDisabledSettingsAfterSettingsWait(t *testing.T) {
	f := newFixture(t)
	f.enable(t, true)
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.batch)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err := tx.Exec(f.ctx, `UPDATE restaurant_settings SET auto_writeoff_expired=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.expiry().Expire(f.ctx) }()
	waitLock(t, f, "%FROM restaurant_settings%FOR SHARE%")
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f.balance(t) != "10.000" || f.expirationCounts(t) != [3]int{} {
		t.Fatal("worker used settings from before lock wait")
	}
}
