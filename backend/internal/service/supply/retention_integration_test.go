package supply

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"go.uber.org/zap"
)

func TestIdempotencyRetentionAndHistoryCleanupIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	s := idempotency.NewService(repository.NewRepository(f.pool, zap.NewNop()))
	if _, err := f.service.Receive(f.ctx, f.actor, uuid.New(), supplyBody(t, f)); err != nil {
		t.Fatal(err)
	}
	scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/orders", Key: uuid.New()}
	operation := func(tx repository.DBTX) (idempotency.Outcome, error) {
		var id uuid.UUID
		err := tx.QueryRow(f.ctx, `INSERT INTO order_attempts (created_by, reason_code, items_snapshot)
			VALUES ($1, 'INSUFFICIENT_STOCK', '[{"portions":1,"dish_name":"Историческое имя"}]'::jsonb)
			RETURNING id`, f.actor.String()).Scan(&id)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		body, err := json.Marshal(map[string]any{"attempt_id": id, "code": "INSUFFICIENT_STOCK"})
		return idempotency.Outcome{Response: model.IdempotencyResponse{Status: http.StatusConflict, Body: body}}, err
	}
	first, err := s.Execute(f.ctx, scope, []byte("order"), operation)
	if err != nil {
		t.Fatal(err)
	}
	var retainedFor24Hours bool
	if err := f.pool.QueryRow(f.ctx, `SELECT bool_and(expires_at >= created_at + interval '24 hours')
		FROM idempotency_keys`).Scan(&retainedFor24Hours); err != nil || !retainedFor24Hours {
		t.Fatalf("completed results must be retained at least 24 hours: %v", err)
	}
	if deleted, err := s.CleanupExpired(f.ctx); err != nil || deleted != 0 {
		t.Fatalf("cleanup removed active results: %d, %v", deleted, err)
	}
	replay, err := s.Execute(f.ctx, scope, []byte("order"), operation)
	if err != nil || !sameResponse(&first, &replay) {
		t.Fatalf("active refusal was not replayed: %v", err)
	}
	var attemptID uuid.UUID
	var snapshot []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT id, items_snapshot FROM order_attempts`).
		Scan(&attemptID, &snapshot); err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE idempotency_keys SET created_at = clock_timestamp() - interval '2 days',
		expires_at = clock_timestamp() WHERE user_id = $1 AND endpoint = $2 AND key = $3`,
		scope.UserID.String(), scope.Endpoint(), scope.Key.String())
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := s.CleanupExpired(f.ctx); err != nil || deleted != 1 {
		t.Fatalf("expected only expired refusal cache to be removed: %d, %v", deleted, err)
	}
	var unchanged bool
	if err := f.pool.QueryRow(f.ctx, `SELECT created_by = $2 AND reason_code = 'INSUFFICIENT_STOCK'
		AND items_snapshot = $3::jsonb FROM order_attempts WHERE id = $1`, attemptID.String(), f.actor.String(), snapshot).
		Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("cleanup changed independent refusal history: %v", err)
	}
	assertSupplyCounts(t, f, 1, 3)
	assertIdempotencyCount(t, f, 1)
	newAttempt, err := s.Execute(f.ctx, scope, []byte("new order after expiry"), operation)
	if err != nil || sameResponse(&first, &newAttempt) {
		t.Fatalf("expired key cannot be reused: %v", err)
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE idempotency_keys SET created_at = clock_timestamp() - interval '2 days',
		expires_at = clock_timestamp() - interval '1 day'`)
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := s.CleanupExpired(f.ctx); err != nil || deleted != 2 {
		t.Fatalf("cleanup did not remove expired cache: %d, %v", deleted, err)
	}
	var attempts int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM order_attempts").
		Scan(&attempts); err != nil ||
		attempts != 2 {
		t.Fatalf("cleanup deleted refusal facts: count %d, %v", attempts, err)
	}
	assertSupplyCounts(t, f, 1, 3)
	assertIdempotencyCount(t, f, 0)
}

func TestIdempotencyCleanupBatchAndLockedRowsIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	shared := repository.NewRepository(f.pool, zap.NewNop())
	s := idempotency.NewService(shared)
	keys := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, key := range keys {
		scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/test", Key: key}
		_, err := s.Execute(f.ctx, scope, []byte("body"), func(repository.DBTX) (idempotency.Outcome, error) {
			return idempotency.Outcome{
				Response: model.IdempotencyResponse{Status: 200, Body: json.RawMessage(`{}`)},
			}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.pool.Exec(f.ctx, `UPDATE idempotency_keys SET created_at = clock_timestamp() - interval '2 days',
		expires_at = clock_timestamp() - interval '1 day' WHERE key <> $1`, keys[2].String())
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(
		f.ctx,
		`SELECT id FROM idempotency_keys WHERE key = $1 FOR UPDATE`,
		keys[0].String(),
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, time.Second)
	defer cancel()
	err = shared.WithinTx(ctx, nil, func(tx repository.DBTX) error {
		deleted, err := shared.DeleteExpiredIdempotencyResults(ctx, tx, 1)
		if err == nil && deleted != 1 {
			t.Errorf("bounded batch should remove one unlocked row, got %d", deleted)
		}
		return err
	})
	if err != nil {
		t.Fatalf("cleanup must skip locked rows: %v", err)
	}
	assertIdempotencyCount(t, f, 2)
	if deleted, err := s.CleanupExpired(ctx); err != nil || deleted != 0 {
		t.Fatalf("locked or active row was removed: %d, %v", deleted, err)
	}
	if err := blocker.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if deleted, err := s.CleanupExpired(f.ctx); err != nil || deleted != 1 {
		t.Fatalf("unlocked expired row was not removed: %d, %v", deleted, err)
	}
	assertIdempotencyCount(t, f, 1)
	assertSupplyCounts(t, f, 0, 0)
}
