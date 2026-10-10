package supply

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	supplyhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type supplyHTTPService struct{ service *Service }

func (s supplyHTTPService) CreateSupplyIdempotent(
	ctx context.Context, actor, key uuid.UUID, body []byte,
) (model.IdempotencyResponse, error) {
	return s.service.Receive(ctx, actor, key, body)
}

func (s supplyHTTPService) GetSupply(ctx context.Context, id uuid.UUID) (dto.SupplyDetail, error) {
	return s.service.Get(ctx, id)
}

type supplyHTTPFixture struct {
	*supplyFixture
	server *httptest.Server
	client *http.Client
	token  string
	csrf   string
}

func newSupplyHTTPFixture(t *testing.T, middleware ...gin.HandlerFunc) *supplyHTTPFixture {
	t.Helper()
	f := &supplyHTTPFixture{
		supplyFixture: newSupplyFixture(t), token: uuid.NewString() + uuid.NewString(),
		csrf: uuid.NewString(),
	}
	_, err := f.pool.Exec(f.ctx, `INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at)
		VALUES ($1, $2, $3, clock_timestamp() + interval '12 hours')`,
		f.actor.String(), authservice.SessionTokenHash(f.token), f.csrf)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authservice.NewService(authrepository.NewRepository(f.pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	logger := zap.NewNop()
	h := basehandler.NewHandler(supplyhandler.NewHandler(supplyHTTPService{f.service}),
		authhandler.NewHandler(auth), logger)
	router := gin.New()
	router.Use(basehandler.RequestID(), basehandler.Recovery(logger), basehandler.RequestLogger(logger),
		basehandler.RateLimit(10, 20), basehandler.BodyLimit(1024*1024))
	router.Use(middleware...)
	basehandler.RegisterRoutes(router.Group("/api/v1"), h)
	router.NoRoute(basehandler.NotFound)
	f.server = httptest.NewServer(router)
	f.client = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	t.Cleanup(func() { f.client.CloseIdleConnections(); f.server.Close() })
	return f
}

type supplyHTTPResult struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func (f *supplyHTTPFixture) request(method, path string, key uuid.UUID, body []byte,
	modify func(*http.Request),
) supplyHTTPResult {
	req, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+path, bytes.NewReader(body))
	if err != nil {
		return supplyHTTPResult{err: err}
	}
	req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key.String())
		req.Header.Set("Origin", "http://flagman.localhost")
		req.Header.Set("X-CSRF-Token", f.csrf)
	}
	if modify != nil {
		modify(req)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return supplyHTTPResult{err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return supplyHTTPResult{status: resp.StatusCode, header: resp.Header, body: data, err: err}
}

func requireSupplyHTTPStatus(t *testing.T, result supplyHTTPResult, status int) {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.status != status {
		t.Fatalf("status %d, want %d; body: %s", result.status, status, result.body)
	}
}

func TestSupplyHTTPDetailsIntegration(t *testing.T) {
	f := newSupplyHTTPFixture(t)
	post := f.request(http.MethodPost, "/api/v1/supplies", uuid.New(), supplyBody(t, f.supplyFixture), nil)
	requireSupplyHTTPStatus(t, post, http.StatusCreated)
	var details dto.SupplyDetail
	if err := json.Unmarshal(post.body, &details); err != nil {
		t.Fatal(err)
	}
	location := "/api/v1/supplies/" + details.ID.String()
	if details.ID == uuid.Nil || post.header.Get("Location") != location || post.header.Get("X-Request-ID") == "" {
		t.Fatal("missing supply identity, Location or request ID")
	}
	if details.Number != "TEST-104" || details.Status != "received" || details.ItemsCount != 3 ||
		len(details.Items) != 3 {
		t.Fatalf("incorrect supply details: %+v", details)
	}
	if details.CreatedBy.ID != f.actor || details.CreatedBy.Name != "Приёмщик" ||
		!details.Meta.ServerTime.Equal(f.service.now()) {
		t.Fatal("incorrect actor or calculation time")
	}
	quantities := []string{"1.250", "200.125", "2.000"}
	for i, item := range details.Items {
		batch := item.Batch
		if batch.ID == uuid.Nil || batch.ProductID.String() != f.input.Items[i].ProductID || batch.LineNumber != i+1 {
			t.Fatalf("incorrect identity or order for item %d", i)
		}
		if batch.InitialQuantity != quantities[i] || batch.RemainingQuantity != quantities[i] {
			t.Fatalf("incorrect quantities for item %d", i)
		}
		if batch.Supply.ID != details.ID || batch.Supply.Number != details.Number ||
			item.Product.ID != batch.ProductID {
			t.Fatalf("incorrect nested references for item %d", i)
		}
		if batch.CreatedAt.IsZero() || details.CreatedAt.IsZero() || batch.CreatedAt.Location() != time.UTC {
			t.Fatal("missing UTC database timestamps")
		}
	}
	calculated := details.Items[1]
	if calculated.Product.Name != "Моцарелла" || calculated.Product.DisplayUnit != "kg" ||
		calculated.Batch.Unit != "g" {
		t.Fatal("incorrect product projection")
	}
	if calculated.Batch.ExpirySource != "calculated" || calculated.Batch.ShelfLifeHours == nil ||
		*calculated.Batch.ShelfLifeHours != 72 || calculated.Batch.ProducedAt == nil || calculated.Batch.Status != "valid" {
		t.Fatal("missing actual calculated shelf life")
	}
	explicit := details.Items[0].Batch
	if explicit.ExpirySource != "explicit" || explicit.ProducedAt != nil || explicit.ShelfLifeHours != nil ||
		explicit.Status != "expiring" || explicit.RemainingSeconds != 24*60*60 {
		t.Fatal("incorrect explicit expiry or threshold boundary")
	}
	get := f.request(http.MethodGet, location, uuid.Nil, nil, nil)
	requireSupplyHTTPStatus(t, get, http.StatusOK)
	var current dto.SupplyDetail
	if err := json.Unmarshal(get.body, &current); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, details) {
		t.Fatal("Location does not return the created supply")
	}
	requireSupplyHTTPStatus(t, f.request(http.MethodGet, "/api/v1/supplies/"+uuid.NewString(), uuid.Nil, nil, nil), 404)
	requireSupplyHTTPStatus(t, f.request(http.MethodGet, "/api/v1/supplies/invalid", uuid.Nil, nil, nil), 422)
	assertSupplyCounts(t, f.supplyFixture, 1, 3)
	assertIdempotencyCount(t, f.supplyFixture, 1)
}

func TestSupplyHTTPDuplicateNumberIntegration(t *testing.T) {
	f := newSupplyHTTPFixture(t)
	body := supplyBody(t, f.supplyFixture)
	requireSupplyHTTPStatus(t, f.request(http.MethodPost, "/api/v1/supplies", uuid.New(), body, nil), 201)
	key := uuid.New()
	duplicate := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	requireSupplyHTTPStatus(t, duplicate, 409)
	if duplicate.header.Get("Location") != "" || !bytes.Contains(duplicate.body, []byte(`"code": "CONFLICT"`)) {
		t.Fatalf("incorrect duplicate response: %s", duplicate.body)
	}
	replay := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	requireSupplyHTTPStatus(t, replay, 409)
	if !bytes.Equal(replay.body, duplicate.body) {
		t.Fatal("duplicate refusal was not replayed")
	}
	assertSupplyCounts(t, f.supplyFixture, 1, 3)
	assertIdempotencyCount(t, f.supplyFixture, 2)
}

func TestSupplyHTTPConcurrentRetryIntegration(t *testing.T) {
	f := newSupplyHTTPFixture(t)
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(f.ctx, "SELECT id FROM products WHERE id = $1 FOR UPDATE", f.low.String()); err != nil {
		t.Fatal(err)
	}
	key, body := uuid.New(), supplyBody(t, f.supplyFixture)
	firstResult := make(chan supplyHTTPResult, 1)
	go func() { firstResult <- f.request(http.MethodPost, "/api/v1/supplies", key, body, nil) }()
	waitForProductLock(t, f.supplyFixture)
	busy := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	requireSupplyHTTPStatus(t, busy, 409)
	if busy.header.Get("Retry-After") != "1" || !bytes.Contains(busy.body, []byte("REQUEST_IN_PROGRESS")) {
		t.Fatal("unfinished retry did not return REQUEST_IN_PROGRESS with Retry-After")
	}
	if err := blocker.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	first := <-firstResult
	requireSupplyHTTPStatus(t, first, 201)
	replay := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	requireSupplyHTTPStatus(t, replay, 201)
	if !bytes.Equal(first.body, replay.body) || first.header.Get("Location") != replay.header.Get("Location") ||
		first.header.Get("X-Request-ID") == replay.header.Get("X-Request-ID") {
		t.Fatal("retry must preserve response and Location with a fresh request ID")
	}
	changedBody := append(append([]byte(nil), body...), ' ')
	conflict := f.request(http.MethodPost, "/api/v1/supplies", key, changedBody, nil)
	requireSupplyHTTPStatus(t, conflict, 409)
	if !bytes.Contains(conflict.body, []byte("IDEMPOTENCY_CONFLICT")) {
		t.Fatal("changed body did not conflict")
	}
	assertSupplyCounts(t, f.supplyFixture, 1, 3)
	assertIdempotencyCount(t, f.supplyFixture, 1)
}

func TestSupplyHTTPConcurrentDuplicateNumberIntegration(t *testing.T) {
	f := newSupplyHTTPFixture(t)
	barrier := &numberBarrierRepository{Repository: f.repo, ready: make(chan struct{}, 2), release: make(chan struct{})}
	f.service.repo = barrier
	results := make(chan supplyHTTPResult, 2)
	for _, item := range []dto.CreateSupplyItem{f.input.Items[0], f.input.Items[1]} {
		input := f.input
		input.Items = []dto.CreateSupplyItem{item}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		go func() { results <- f.request(http.MethodPost, "/api/v1/supplies", uuid.New(), body, nil) }()
	}
	for range 2 {
		select {
		case <-barrier.ready:
		case <-f.ctx.Done():
			t.Fatal("prechecks did not run concurrently")
		}
	}
	close(barrier.release)
	statuses := map[int]int{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		statuses[result.status]++
	}
	if statuses[201] != 1 || statuses[409] != 1 {
		t.Fatalf("unexpected concurrent statuses: %v", statuses)
	}
	assertSupplyCounts(t, f.supplyFixture, 1, 1)
	assertIdempotencyCount(t, f.supplyFixture, 2)
}

func TestSupplyHTTPAtomicityIntegration(t *testing.T) {
	for _, failure := range []string{"invalid last item", "outbox", "projection", "commit"} {
		t.Run(failure, func(t *testing.T) {
			f := newSupplyHTTPFixture(t)
			status, keys := 500, 0
			var setup string
			switch failure {
			case "invalid last item":
				expired := f.service.now().Add(-time.Second).Format(time.RFC3339Nano)
				f.input.Items[2].ExpiresAt = &expired
				status, keys = 422, 1
			case "outbox":
				setup = "ALTER TABLE outbox_events ADD CONSTRAINT forced_failure CHECK (false)"
			case "projection":
				setup = "ALTER TABLE users RENAME COLUMN name TO hidden_name"
			case "commit":
				setup = `CREATE FUNCTION reject_http_commit() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'forced commit failure'; END; $$;
				CREATE CONSTRAINT TRIGGER reject_http_commit AFTER INSERT ON idempotency_keys
				DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_http_commit();`
			}
			if setup != "" {
				if _, err := f.pool.Exec(f.ctx, setup); err != nil {
					t.Fatal(err)
				}
			}
			result := f.request(http.MethodPost, "/api/v1/supplies", uuid.New(), supplyBody(t, f.supplyFixture), nil)
			requireSupplyHTTPStatus(t, result, status)
			if result.header.Get("Location") != "" || bytes.Contains(result.body, []byte("forced")) ||
				bytes.Contains(result.body, []byte("SQLSTATE")) {
				t.Fatal("failure exposes Location or database details")
			}
			assertSupplyCounts(t, f.supplyFixture, 0, 0)
			assertIdempotencyCount(t, f.supplyFixture, keys)
		})
	}
}

// Disconnect only after the successful transaction, before its response body reaches the client.
type lostSupplyResponseWriter struct {
	gin.ResponseWriter
	drop *atomic.Bool
}

func (w *lostSupplyResponseWriter) Write(body []byte) (int, error) {
	if w.Status() == http.StatusCreated && w.drop.CompareAndSwap(true, false) {
		conn, _, err := w.ResponseWriter.Hijack()
		if err != nil {
			return 0, err
		}
		_ = conn.Close()
		return 0, net.ErrClosed
	}
	return w.ResponseWriter.Write(body)
}

func TestSupplyHTTPLostResponseReplayIntegration(t *testing.T) {
	var drop atomic.Bool
	drop.Store(true)
	f := newSupplyHTTPFixture(t, func(c *gin.Context) {
		c.Writer = &lostSupplyResponseWriter{ResponseWriter: c.Writer, drop: &drop}
		c.Next()
	})
	key, body := uuid.New(), supplyBody(t, f.supplyFixture)
	lost := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	if lost.err == nil {
		t.Fatal("first response unexpectedly reached the client")
	}
	var saved model.IdempotencyResponse
	if err := f.pool.QueryRow(f.ctx, `SELECT response_status, response_body, response_headers->>'Location'
		FROM idempotency_keys WHERE user_id = $1 AND key = $2`, f.actor.String(), key.String()).
		Scan(&saved.Status, &saved.Body, &saved.Location); err != nil {
		t.Fatal(err)
	}
	assertSupplyCounts(t, f.supplyFixture, 1, 3)
	replay := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
	requireSupplyHTTPStatus(t, replay, 201)
	if saved.Status != replay.status || !bytes.Equal(saved.Body, replay.body) ||
		saved.Location != replay.header.Get("Location") {
		t.Fatal("network retry did not return the exact committed response")
	}
	assertSupplyCounts(t, f.supplyFixture, 1, 3)
	assertIdempotencyCount(t, f.supplyFixture, 1)
}

func TestSupplyHTTPAccessIntegration(t *testing.T) {
	for _, test := range []struct {
		name, setup string
		status      int
		modify      func(*http.Request)
	}{
		{name: "no session", status: 401, modify: func(r *http.Request) { r.Header.Del("Cookie"); r.Header.Set("X-User-ID", uuid.NewString()) }},
		{name: "unknown session", status: 401, modify: func(r *http.Request) {
			r.Header.Set("Cookie", authservice.SessionCookieName+"="+strings.Repeat("x", 64))
			r.Header.Del("X-CSRF-Token")
		}},
		{name: "expired session", status: 401, setup: `UPDATE sessions
			SET created_at = clock_timestamp() - interval '1 day', expires_at = clock_timestamp() - interval '1 second'`},
		{name: "revoked session", status: 401, setup: "UPDATE sessions SET revoked_at = clock_timestamp()"},
		{name: "inactive actor", status: 401, setup: "UPDATE users SET is_active = false"},
		{name: "operator", status: 403, setup: "UPDATE users SET role = 'order_operator'"},
		{name: "missing csrf", status: 403, modify: func(r *http.Request) { r.Header.Del("X-CSRF-Token") }},
		{name: "wrong csrf", status: 403, modify: func(r *http.Request) { r.Header.Set("X-CSRF-Token", "invalid") }},
		{name: "foreign origin", status: 403, modify: func(r *http.Request) { r.Header.Set("Origin", "http://foreign.test") }},
		{name: "ambiguous csrf", status: 403, modify: func(r *http.Request) { r.Header.Add("X-CSRF-Token", "invalid") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSupplyHTTPFixture(t)
			if test.setup != "" {
				if _, err := f.pool.Exec(f.ctx, test.setup); err != nil {
					t.Fatal(err)
				}
			}
			requireSupplyHTTPStatus(
				t,
				f.request(http.MethodPost, "/api/v1/supplies", uuid.New(), supplyBody(t, f.supplyFixture), test.modify),
				test.status,
			)
			assertSupplyCounts(t, f.supplyFixture, 0, 0)
			assertIdempotencyCount(t, f.supplyFixture, 0)
		})
	}
}

func TestSupplyHTTPEarlyFailureDoesNotConsumeKeyIntegration(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   func([]byte) []byte
		modify func(*http.Request)
	}{
		{name: "malformed JSON", status: 400, body: func([]byte) []byte { return []byte(`{"number":`) }},
		{name: "unknown JSON field", status: 400, body: func([]byte) []byte { return []byte(`{"actor_id":"forged"}`) }},
		{name: "quantity validation", status: 422, body: func(body []byte) []byte {
			return bytes.Replace(body, []byte(`"quantity":"1.250"`), []byte(`"quantity":"0"`), 1)
		}},
		{name: "query validation", status: 422, modify: func(r *http.Request) { r.URL.RawQuery = "unknown=true" }},
		{name: "key validation", status: 422, modify: func(r *http.Request) { r.Header.Set("Idempotency-Key", "invalid") }},
		{name: "content type", status: 400, modify: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{name: "no access", status: 401, modify: func(r *http.Request) { r.Header.Del("Cookie") }},
		{name: "wrong csrf", status: 403, modify: func(r *http.Request) { r.Header.Set("X-CSRF-Token", "invalid") }},
		{name: "wrong origin", status: 403, modify: func(r *http.Request) { r.Header.Set("Origin", "http://foreign.test") }},
		{name: "body limit", status: 413, body: func([]byte) []byte { return bytes.Repeat([]byte(" "), 1024*1024+1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSupplyHTTPFixture(t)
			key, body := uuid.New(), supplyBody(t, f.supplyFixture)
			invalidBody := body
			if test.body != nil {
				invalidBody = test.body(body)
			}
			failure := f.request(http.MethodPost, "/api/v1/supplies", key, invalidBody, test.modify)
			requireSupplyHTTPStatus(t, failure, test.status)
			assertIdempotencyCount(t, f.supplyFixture, 0)
			assertSupplyCounts(t, f.supplyFixture, 0, 0)
			// Correcting an early rejected request may safely use the same key.
			fixed := f.request(http.MethodPost, "/api/v1/supplies", key, body, nil)
			requireSupplyHTTPStatus(t, fixed, http.StatusCreated)
			assertIdempotencyCount(t, f.supplyFixture, 1)
			assertSupplyCounts(t, f.supplyFixture, 1, 3)
		})
	}
}
