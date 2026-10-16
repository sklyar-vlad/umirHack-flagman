package supply

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"go.uber.org/zap"
)

func renderTestSupply(ctx context.Context, tx repository.DBTX, created *model.CreatedSupply) (json.RawMessage, error) {
	var number string
	if err := tx.QueryRow(ctx, `SELECT number FROM supplies WHERE id = $1`, created.ID.String()).
		Scan(&number); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		ID     uuid.UUID `json:"id"`
		Number string    `json:"number"`
	}{ID: created.ID, Number: number})
}

func supplyBody(t *testing.T, f *supplyFixture) []byte {
	t.Helper()
	body, err := json.Marshal(f.input)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func assertIdempotencyCount(t *testing.T, f *supplyFixture, want int) {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM idempotency_keys`).
		Scan(&count); err != nil ||
		count != want {
		t.Fatalf("idempotency records: got %d, want %d, error %v", count, want, err)
	}
}

func sameResponse(a, b *model.IdempotencyResponse) bool {
	return a.Status == b.Status && a.Location == b.Location && bytes.Equal(a.Body, b.Body)
}

func TestSupplyIdempotencyReplayIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	key := uuid.New()
	body := supplyBody(t, f)
	first, err := f.service.CreateIdempotent(f.ctx, f.actor, key, body, renderTestSupply)
	if err != nil || first.Status != http.StatusCreated || first.Location == "" {
		t.Fatalf("first response: %+v, %v", first, err)
	}
	assertSupplyCounts(t, f, 1, 3)
	assertIdempotencyCount(t, f, 1)
	var hash, endpoint string
	var userID, storedKey uuid.UUID
	var headerJSON []byte
	err = f.pool.QueryRow(f.ctx, `SELECT user_id, endpoint, key, request_hash, response_headers FROM idempotency_keys`).
		Scan(&userID, &endpoint, &storedKey, &hash, &headerJSON)
	wantHash := sha256.Sum256(body)
	if err != nil || userID != f.actor || storedKey != key || endpoint != "POST /api/v1/supplies" ||
		hash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("incorrect stored scope/hash: %v", err)
	}
	var headers map[string]string
	if err := json.Unmarshal(
		headerJSON,
		&headers,
	); err != nil || len(headers) != 1 ||
		headers["Location"] != first.Location {
		t.Fatalf("headers must contain only Location: %s, %v", headerJSON, err)
	}

	if _, err := f.pool.Exec(f.ctx, `UPDATE products SET is_active = false`); err != nil {
		t.Fatal(err)
	}
	now := f.service.now().Add(100 * time.Hour)
	f.service.now = func() time.Time { return now }
	neverRender := func(context.Context, repository.DBTX, *model.CreatedSupply) (json.RawMessage, error) {
		return nil, errors.New("replay called the operation")
	}
	second, err := f.service.CreateIdempotent(
		requestctx.WithID(f.ctx, "retry-request"),
		f.actor,
		key,
		body,
		neverRender,
	)
	if err != nil || !sameResponse(&first, &second) {
		t.Fatalf("replay differs: %+v, %v", second, err)
	}
	for _, changed := range [][]byte{append(bytes.Clone(body), ' '), bytes.Replace(body, []byte("TEST-104"), []byte("TEST-105"), 1)} {
		response, err := f.service.CreateIdempotent(f.ctx, f.actor, key, changed, neverRender)
		if !errors.Is(err, apperrors.ErrIdempotencyConflict) || response.Status != 0 {
			t.Fatalf("different body should conflict: %+v, %v", response, err)
		}
	}
	assertSupplyCounts(t, f, 1, 3)
	assertIdempotencyCount(t, f, 1)
}

func TestSupplyIdempotencyBeforeProductLocksIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(f.ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE`, f.high.String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	key := uuid.New()
	body := supplyBody(t, f)
	type result struct {
		response model.IdempotencyResponse
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := f.service.CreateIdempotent(ctx, f.actor, key, body, renderTestSupply)
		done <- result{response, err}
	}()
	waitForProductLock(t, f)
	assertIdempotencyCount(t, f, 0)
	for _, parallelBody := range [][]byte{body, append(bytes.Clone(body), ' ')} {
		parallelCtx, parallelCancel := context.WithTimeout(f.ctx, time.Second)
		response, err := f.service.CreateIdempotent(parallelCtx, f.actor, key, parallelBody, renderTestSupply)
		parallelCancel()
		if !errors.Is(err, apperrors.ErrRequestInProgress) || response.Status != 0 {
			t.Fatalf("parallel call must fail before waiting for products: %+v, %v", response, err)
		}
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	first := <-done
	if first.err != nil {
		t.Fatal(first.err)
	}
	retry, err := f.service.CreateIdempotent(f.ctx, f.actor, key, body, renderTestSupply)
	if err != nil || !sameResponse(&first.response, &retry) {
		t.Fatalf("completed retry failed: %v", err)
	}
	assertSupplyCounts(t, f, 1, 3)
	assertIdempotencyCount(t, f, 1)
}

type noNumberPrecheckRepository struct{ Repository }

func (r *noNumberPrecheckRepository) NumberExists(context.Context, repository.DBTX, string) (bool, error) {
	return false, nil
}

func TestSupplyIdempotencyRefusalIntegration(t *testing.T) {
	for _, kind := range []string{"validation", "SQL conflict"} {
		t.Run(kind, func(t *testing.T) {
			f := newSupplyFixture(t)
			var status, supplyCount, batchCount int
			if kind == "validation" {
				expires := f.service.now().Add(-time.Minute).Format(time.RFC3339Nano)
				f.input.Items[2].ExpiresAt = &expires
				status = http.StatusUnprocessableEntity
			} else {
				if _, err := f.service.Create(f.ctx, f.actor, f.input); err != nil {
					t.Fatal(err)
				}
				shared := repository.NewRepository(f.pool, zap.NewNop())
				f.service.repo = &noNumberPrecheckRepository{Repository: f.repo}
				f.service.idempotency = idempotency.NewService(shared)
				status, supplyCount, batchCount = http.StatusConflict, 1, 3
			}
			body := supplyBody(t, f)
			key := uuid.New()
			first, err := f.service.CreateIdempotent(f.ctx, f.actor, key, body, renderTestSupply)
			if err != nil || first.Status != status || first.Location != "" {
				t.Fatalf("domain refusal not stored: %+v, %v", first, err)
			}
			second, err := f.service.CreateIdempotent(
				requestctx.WithID(f.ctx, "refusal-retry"),
				f.actor,
				key,
				body,
				renderTestSupply,
			)
			if err != nil || !sameResponse(&first, &second) {
				t.Fatalf("refusal replay mismatch: %v", err)
			}
			var response dto.ErrorResponse
			if err := json.Unmarshal(second.Body, &response); err != nil || response.RequestID != "supply-integration" {
				t.Fatalf("saved response must retain original request_id: %s, %v", second.Body, err)
			}
			assertSupplyCounts(t, f, supplyCount, batchCount)
			assertIdempotencyCount(t, f, 1)
		})
	}
}

func TestSupplyIdempotencyTechnicalRollbackIntegration(t *testing.T) {
	for _, failure := range []string{"outbox", "result storage", "commit", "renderer"} {
		t.Run(failure, func(t *testing.T) {
			f := newSupplyFixture(t)
			render := renderTestSupply
			var setup, restore string
			switch failure {
			case "outbox":
				setup = "ALTER TABLE outbox_events ADD CONSTRAINT forced_failure CHECK (false)"
				restore = "ALTER TABLE outbox_events DROP CONSTRAINT forced_failure"
			case "result storage":
				setup = "ALTER TABLE idempotency_keys ADD CONSTRAINT forced_failure CHECK (false)"
				restore = "ALTER TABLE idempotency_keys DROP CONSTRAINT forced_failure"
			case "commit":
				setup = `CREATE FUNCTION reject_idempotency_commit() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'forced commit failure'; END; $$;
				CREATE CONSTRAINT TRIGGER reject_idempotency_commit AFTER INSERT ON idempotency_keys
				DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_idempotency_commit();`
				restore = "DROP TRIGGER reject_idempotency_commit ON idempotency_keys"
			case "renderer":
				render = func(context.Context, repository.DBTX, *model.CreatedSupply) (json.RawMessage, error) {
					return nil, errors.New("forced serialization failure")
				}
			}
			if setup != "" {
				if _, err := f.pool.Exec(f.ctx, setup); err != nil {
					t.Fatal(err)
				}
			}
			key, body := uuid.New(), supplyBody(t, f)
			response, err := f.service.CreateIdempotent(f.ctx, f.actor, key, body, render)
			if err == nil || response.Status != 0 {
				t.Fatalf("technical failure returned completed response: %+v, %v", response, err)
			}
			assertSupplyCounts(t, f, 0, 0)
			assertIdempotencyCount(t, f, 0)
			if restore != "" {
				if _, err := f.pool.Exec(f.ctx, restore); err != nil {
					t.Fatal(err)
				}
			}
			response, err = f.service.CreateIdempotent(f.ctx, f.actor, key, body, renderTestSupply)
			if err != nil || response.Status != http.StatusCreated {
				t.Fatalf("key was not released for a safe retry: %v", err)
			}
			assertSupplyCounts(t, f, 1, 3)
			assertIdempotencyCount(t, f, 1)
		})
	}
}

func TestIdempotencyScopesAndExpiryIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	shared := repository.NewRepository(f.pool, zap.NewNop())
	s := idempotency.NewService(shared)
	scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/supplies", Key: uuid.New()}
	var calls int
	operation := func(repository.DBTX) (idempotency.Outcome, error) {
		calls++
		return idempotency.Outcome{Response: model.IdempotencyResponse{
			Status: http.StatusCreated, Body: json.RawMessage(fmt.Sprintf(`{"operation":%d}`, calls)),
			Location: fmt.Sprintf("/api/v1/results/%d", calls),
		}}, nil
	}
	first, err := s.Execute(f.ctx, scope, []byte("original"), operation)
	if err != nil {
		t.Fatal(err)
	}
	otherUser := uuid.New()
	_, err = f.pool.Exec(f.ctx, `INSERT INTO users (id, name, email, password_hash, role)
		VALUES ($1, 'Другой', 'other@example.test', 'fixture-hash', 'admin')`, otherUser.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*model.IdempotencyScope){
		func(s *model.IdempotencyScope) { s.UserID = otherUser },
		func(s *model.IdempotencyScope) { s.Method = http.MethodPut },
		func(s *model.IdempotencyScope) { s.Path = "/api/v1/orders" },
		func(s *model.IdempotencyScope) { s.Key = uuid.New() },
	} {
		other := scope
		change(&other)
		if response, err := s.Execute(
			f.ctx,
			other,
			[]byte("different"),
			operation,
		); err != nil ||
			sameResponse(&first, &response) {
			t.Fatalf("scope leaked into another request: %v", err)
		}
	}
	assertIdempotencyCount(t, f, 5)
	if calls != 5 {
		t.Fatalf("expected independent operations, got %d", calls)
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE idempotency_keys
		SET created_at = clock_timestamp() - INTERVAL '2 days', expires_at = clock_timestamp() - INTERVAL '1 day'
		WHERE user_id = $1 AND endpoint = $2 AND key = $3`, scope.UserID.String(), scope.Endpoint(), scope.Key.String())
	if err != nil {
		t.Fatal(err)
	}
	response, err := s.Execute(f.ctx, scope, []byte("new body after expiry"), operation)
	if err != nil || sameResponse(&first, &response) || calls != 6 {
		t.Fatalf("expired key not replaced: %+v, %v", response, err)
	}
	assertIdempotencyCount(t, f, 5)
	assertSupplyCounts(t, f, 0, 0)
}

func TestIdempotencyCompletedRefusalRetainsFactsIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	s := idempotency.NewService(repository.NewRepository(f.pool, zap.NewNop()))
	scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/orders", Key: uuid.New()}
	operation := func(tx repository.DBTX) (idempotency.Outcome, error) {
		_, err := tx.Exec(f.ctx, `INSERT INTO order_attempts (created_by, reason_code, items_snapshot)
			VALUES ($1, 'INSUFFICIENT_STOCK', '[{"portions":1}]'::jsonb)`, f.actor.String())
		if err != nil {
			return idempotency.Outcome{}, err
		}
		return idempotency.Outcome{Response: model.IdempotencyResponse{
			Status: http.StatusConflict, Body: json.RawMessage(`{"error":{"code":"INSUFFICIENT_STOCK"}}`),
		}}, nil
	}
	first, err := s.Execute(f.ctx, scope, []byte("order"), operation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Execute(f.ctx, scope, []byte("order"), operation)
	if err != nil || !sameResponse(&first, &second) {
		t.Fatalf("refusal replay failed: %v", err)
	}
	var attempts int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM order_attempts`).
		Scan(&attempts); err != nil ||
		attempts != 1 {
		t.Fatalf("refusal facts repeated or rolled back: %d, %v", attempts, err)
	}
	assertIdempotencyCount(t, f, 1)
}

func TestIdempotencyCancellationIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	s := idempotency.NewService(repository.NewRepository(f.pool, zap.NewNop()))
	scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/test", Key: uuid.New()}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	_, err := s.Execute(ctx, scope, []byte("test"), func(tx repository.DBTX) (idempotency.Outcome, error) {
		if _, err := tx.Exec(ctx, `INSERT INTO product_categories (name) VALUES ('Отменённая запись')`); err != nil {
			return idempotency.Outcome{}, err
		}
		cancel()
		return idempotency.Outcome{
			Response: model.IdempotencyResponse{Status: http.StatusOK, Body: json.RawMessage(`{}`)},
		}, nil
	})
	if err == nil {
		t.Fatal("canceled request must not complete")
	}
	assertIdempotencyCount(t, f, 0)
	var categories int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_categories`).
		Scan(&categories); err != nil ||
		categories != 1 {
		t.Fatalf("canceled writes survived: %v", err)
	}
	_, err = s.Execute(f.ctx, scope, []byte("test"), func(repository.DBTX) (idempotency.Outcome, error) {
		return idempotency.Outcome{
			Response: model.IdempotencyResponse{Status: http.StatusOK, Body: json.RawMessage(`{}`)},
		}, nil
	})
	if err != nil {
		t.Fatalf("canceled request did not release its key: %v", err)
	}
	assertIdempotencyCount(t, f, 1)
}

func TestIdempotencyRejectsUnfinishedResponsesIntegration(t *testing.T) {
	for _, response := range []model.IdempotencyResponse{
		{Status: http.StatusInternalServerError, Body: json.RawMessage(`{}`)},
		{Status: http.StatusCreated, Body: json.RawMessage(`invalid`)},
		{Status: http.StatusCreated, Body: json.RawMessage(`{}`), Location: "/test\r\nSet-Cookie: secret"},
	} {
		f := newSupplyFixture(t)
		s := idempotency.NewService(repository.NewRepository(f.pool, zap.NewNop()))
		scope := model.IdempotencyScope{UserID: f.actor, Method: http.MethodPost, Path: "/api/v1/test", Key: uuid.New()}
		_, err := s.Execute(f.ctx, scope, []byte("test"), func(tx repository.DBTX) (idempotency.Outcome, error) {
			if _, err := tx.Exec(f.ctx, `INSERT INTO product_categories (name) VALUES ('Неполный ответ')`); err != nil {
				return idempotency.Outcome{}, err
			}
			return idempotency.Outcome{Response: response}, nil
		})
		if err == nil {
			t.Fatalf("unfinished or invalid response accepted: %+v", response)
		}
		assertIdempotencyCount(t, f, 0)
		var count int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_categories`).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("operation with invalid response committed: %v", err)
		}
	}
}
