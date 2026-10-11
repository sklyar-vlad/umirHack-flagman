package writeoff_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	writeoffhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestWriteOffReplayConflictAndTypedEvents(t *testing.T) {
	f := newFixture(t)
	key := uuid.New()
	ctx := requestctx.WithID(f.actor(), "writeoff-request")
	r, err := f.writeoff.CreateIdempotent(ctx, f.user, key, f.body("1.001"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE products SET name='Renamed' WHERE id=$1`, f.product)
	again, err := f.writeoff.CreateIdempotent(ctx, f.user, key, f.body("1.001"))
	if err != nil || !bytes.Equal(r.Body, again.Body) || r.Status != again.Status || f.counts(t) != [4]int{1, 1, 1, 1} || f.balance(t) != "8.999" {
		t.Fatalf("replay: %v %v", again, err)
	}
	_, err = f.writeoff.CreateIdempotent(ctx, f.user, key, f.body("2"))
	if !errors.Is(err, apperrors.ErrIdempotencyConflict) {
		t.Fatalf("conflict %v", err)
	}
	var actor, entity uuid.UUID
	var action, typ, rid string
	var before, after []byte
	if err = f.pool.QueryRow(ctx, `SELECT actor_id,entity_id,action,entity_type,request_id,before_data,after_data FROM audit_log`).Scan(&actor, &entity, &action, &typ, &rid, &before, &after); err != nil {
		t.Fatal(err)
	}
	var m dto.StockMovement
	_ = json.Unmarshal(r.Body, &m)
	var snapshot model.StockMovementAuditSnapshot
	if err = json.Unmarshal(after, &snapshot); err != nil {
		t.Fatal(err)
	}
	if actor != f.user || entity != m.ID || action != "manual_writeoff" || typ != "stock_movement" || rid != "writeoff-request" || before != nil || snapshot.BatchID != m.BatchID || snapshot.DeltaQuantity != m.DeltaQuantity || snapshot.RemainingQuantityAfter != m.RemainingQuantityAfter || snapshot.ProductNameSnapshot != "Молоко" || snapshot.UnitSnapshot != "ml" || *snapshot.Note != "Порча продукта" {
		t.Fatalf("audit %s", after)
	}
	var payload []byte
	var eventType string
	if err = f.pool.QueryRow(ctx, `SELECT event_type,payload FROM outbox_events`).Scan(&eventType, &payload); err != nil {
		t.Fatal(err)
	}
	if eventType != "inventory.changed" {
		t.Fatal("event type", eventType)
	}
	for _, want := range []string{"stock-movements", "dashboard", "hot-offers", "procurement-recommendations", f.batch.String(), f.product.String()} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Fatalf("outbox missing %s: %s", want, payload)
		}
	}
	key = uuid.New()
	refusal := f.create(t, "99", key)
	f.exec(t, `UPDATE batches SET initial_quantity=100,remaining_quantity=100 WHERE id=$1`, f.batch)
	replay := f.create(t, "99", key)
	if refusal.Status != 409 || !bytes.Equal(refusal.Body, replay.Body) || f.balance(t) != "100.000" || f.counts(t) != [4]int{1, 1, 1, 2} {
		t.Fatal("refusal not retained")
	}
}
func TestWriteOffImmediateAndDeferredFailuresRollbackAndRetry(t *testing.T) {
	for _, table := range []string{"batches", "stock_movements", "audit_log", "outbox_events", "idempotency_keys"} {
		for _, deferred := range []bool{false, true} {
			t.Run(table+map[bool]string{false: " immediate", true: " deferred"}[deferred], func(t *testing.T) {
				f := newFixture(t)
				key := uuid.New()
				f.exec(t, `CREATE FUNCTION reject_writeoff() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private storage test secret'; END $$`)
				event := "INSERT"
				if table == "batches" {
					event = "UPDATE"
				}
				if deferred {
					f.exec(t, "CREATE CONSTRAINT TRIGGER reject_writeoff AFTER "+event+" ON "+table+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_writeoff()")
				} else {
					f.exec(t, "CREATE TRIGGER reject_writeoff BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_writeoff()")
				}
				r, err := f.writeoff.CreateIdempotent(f.actor(), f.user, key, f.body("1"))
				if err == nil || r.Status != 0 || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "SQLSTATE P0001") || f.balance(t) != "10.000" || f.counts(t) != [4]int{} {
					t.Fatalf("rollback %s %v %v %v", f.balance(t), f.counts(t), r, err)
				}
				f.exec(t, "DROP TRIGGER reject_writeoff ON "+table)
				if r = f.create(t, "1", key); r.Status != 201 || f.counts(t) != [4]int{1, 1, 1, 1} {
					t.Fatalf("retry %v", r)
				}
			})
		}
	}
}
func waitLock(t *testing.T, f *fixture, query string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE $2`, f.schema, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected lock wait")
}
func TestWriteOffIdempotencyInProgressAndFreshActor(t *testing.T) {
	for _, test := range []string{"in_progress", "disabled", "role_changed"} {
		t.Run(test, func(t *testing.T) {
			f := newFixture(t)
			key := uuid.New()
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			if test == "in_progress" {
				f.exec(t, `UPDATE products SET name='Renamed' WHERE id=$1`, f.product)
				_, err = tx.Exec(f.ctx, `SELECT id FROM products WHERE id=$1 FOR UPDATE`, f.product)
			} else {
				_, err = tx.Exec(f.ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.user)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { _, err := f.writeoff.CreateIdempotent(f.actor(), f.user, key, f.body("1")); result <- err }()
			query := "%FROM users%FOR SHARE%"
			if test == "in_progress" {
				query = "%FROM products%FOR UPDATE%"
			}
			waitLock(t, f, query)
			if test == "in_progress" {
				_, err = f.writeoff.CreateIdempotent(f.actor(), f.user, key, f.body("1"))
				if !errors.Is(err, apperrors.ErrRequestInProgress) {
					t.Fatalf("running key %v", err)
				}
			} else {
				sql := `UPDATE users SET is_active=false WHERE id=$1`
				if test == "role_changed" {
					sql = `UPDATE users SET role='order_operator' WHERE id=$1`
				}
				if _, err = tx.Exec(f.ctx, sql, f.user); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			err = <-result
			if test == "in_progress" {
				if err != nil || f.counts(t) != [4]int{1, 1, 1, 1} {
					t.Fatal(err)
				}
			} else if !errors.Is(err, apperrors.ErrForbidden) || f.counts(t) != [4]int{} || f.balance(t) != "10.000" {
				t.Fatalf("stale actor %v %v", err, f.counts(t))
			}
		})
	}
}
func TestWriteOffFailureHTTPLogsOnlySQLState(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `CREATE FUNCTION reject_writeoff_log() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private storage test secret'; END $$`)
	f.exec(t, `CREATE TRIGGER reject_writeoff_log BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_writeoff_log()`)
	core, logs := observer.New(zap.DebugLevel)
	r := gin.New()
	r.Use(basehandler.RequestID(), basehandler.RequestLogger(zap.New(core)))
	access := authhandler.NewHandler(f.service)
	api := r.Group("/api/v1", access.RequireRouteAccess)
	basehandler.RegisterWriteOffRoutes(api, writeoffhandler.NewHandler(f.writeoff))
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	w := request(r, "POST", "/write-offs", string(f.body("1")), token, session.CSRFToken, "https://flagman.example")
	var fields []map[string]any
	for _, entry := range logs.All() {
		fields = append(fields, entry.ContextMap())
	}
	raw, _ := json.Marshal(fields)
	if w.Code != 500 || bytes.Contains(raw, []byte("private storage test secret")) || bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte(f.hash)) || !bytes.Contains(raw, []byte("SQLSTATE P0001")) {
		t.Fatalf("unsafe logs: %d", w.Code)
	}
}
