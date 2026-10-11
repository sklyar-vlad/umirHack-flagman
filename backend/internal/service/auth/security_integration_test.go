package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionHTTPDisabledExpiryAndNoUnauthorizedWrites(t *testing.T) {
	f := newFixture(t)
	router := protectedRouter(f)
	login := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	response := decodeAuth(t, login)
	cookie := login.Result().Cookies()[0]
	f.exec(t, `UPDATE users SET is_active=false WHERE id=$1`, f.user)
	for _, route := range []struct{ method, path string }{{"GET", "/auth/me"}, {"GET", "/dashboard"}, {"POST", "/orders"}, {"POST", "/auth/logout"}} {
		w := authRequest(router, route.method, route.path, "", "https://flagman.example", cookie, response.CSRFToken)
		if w.Code != 401 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("disabled user allowed %s", route.path)
		}
	}
	if w := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, ""); w.Code != 401 {
		t.Fatal("disabled user logged in")
	}
	f.exec(t, `UPDATE users SET is_active=true WHERE id=$1`, f.user)
	login = authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	decodeAuth(t, login)
	cookie = login.Result().Cookies()[0]
	f.exec(t, `UPDATE sessions SET expires_at=clock_timestamp() WHERE token_hash=$1`, authservice.SessionTokenHash(cookie.Value))
	if w := authRequest(router, "GET", "/auth/me", "", "", cookie, ""); w.Code != 401 {
		t.Fatal("expiry boundary accepted")
	}
	var orders, movements int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM stock_movements)`).Scan(&orders, &movements); err != nil {
		t.Fatal(err)
	}
	if orders != 0 || movements != 0 {
		t.Fatal("unauthorized actions changed stock")
	}
}

func TestSessionIssuanceRechecksUserAfterLockWait(t *testing.T) {
	for _, change := range []string{"is_active=false", "password_hash='changed'"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(f.ctx, `UPDATE users SET `+change+` WHERE id=$1`, f.user); err != nil {
				t.Fatal(err)
			}
			conn, err := f.pool.Acquire(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			var pid int
			if err := conn.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, err := authrepository.NewRepository(conn).CreateSession(f.ctx, f.user, f.hash, authservice.SessionTokenHash("concurrent-token"), "csrf")
				result <- err
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				var waiting bool
				if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("session issuance did not wait for user lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Fatal("stale user credentials issued a session")
			}
			var count int
			if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("rejected session persisted")
			}
		})
	}
}

func TestAuthLogsNeverContainCredentialsOrTokens(t *testing.T) {
	f := newFixture(t)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	router := gin.New()
	access := authhandler.NewHandler(f.service)
	router.Use(basehandler.RequestID(), basehandler.RequestLogger(logger), basehandler.Recovery(logger), access.CORS())
	basehandler.RegisterAuthRoutes(router.Group("/api/v1", access.RequireRouteAccess), access)
	login := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	response := decodeAuth(t, login)
	cookie := login.Result().Cookies()[0]
	authRequest(router, "GET", "/auth/me", "", "", cookie, "")
	authRequest(router, "POST", "/auth/logout", "", "https://flagman.example", cookie, "wrong-csrf-marker")
	authRequest(router, "POST", "/auth/login", `{"email":"manager@example.test","password":"wrong-password-marker"}`, "https://flagman.example", nil, "")
	// A database exception must not be copied to request logs or HTTP errors.
	f.exec(t, `CREATE FUNCTION fail_auth_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'credential-leak-marker: %', NEW.csrf_token; END; $$;
 CREATE TRIGGER fail_auth_insert BEFORE INSERT ON sessions FOR EACH ROW EXECUTE FUNCTION fail_auth_insert()`)
	failure := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	if failure.Code != 500 || len(failure.Result().Cookies()) != 0 {
		t.Fatal("failed login was not rejected")
	}
	if logs.Len() != 5 {
		t.Fatalf("missing request logs: %d", logs.Len())
	}
	for _, entry := range logs.All() {
		data, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{f.password, f.hash, cookie.Value, response.CSRFToken, "wrong-password-marker", "wrong-csrf-marker", "credential-leak-marker"} {
			if strings.Contains(string(data), secret) || strings.Contains(failure.Body.String(), secret) {
				t.Fatal("authentication secret leaked")
			}
		}
	}
	// Duplicate cookie authentication is rejected before logout can revoke anything.
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://flagman.example")
	req.Header.Set("X-CSRF-Token", response.CSRFToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("duplicate cookie accepted")
	}
	if _, err := f.service.CurrentSession(f.ctx, cookie.Value); err != nil {
		t.Fatal("failed logout revoked a session")
	}
}
