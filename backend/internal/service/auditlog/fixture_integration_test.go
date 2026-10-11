package auditlog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	auditloghandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auditlog"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	settingshandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/settings"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	auditlogrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auditlog"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	settingsrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/settings"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	auditlogservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auditlog"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	settingsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/settings"
	"go.uber.org/zap"
)

type fixture struct {
	schema         string
	ctx            context.Context
	pool           *pgxpool.Pool
	repo           *authrepository.Repository
	service        *authservice.Service
	user           uuid.UUID
	audit          *auditlogservice.Service
	settings       *settingsservice.Service
	password, hash string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("FLAGMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FLAGMAN_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	if (cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1" && cfg.ConnConfig.Host != "localhost") || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("auth tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	name := "auditlog_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg = cfg.Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("missing migrations")
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{schema: name, ctx: ctx, pool: pool, user: uuid.New(), password: "correct test password"}
	f.hash, err = authservice.Hash(f.password)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Test Manager','manager@example.test',$2,'admin')`, f.user, f.hash)
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name) VALUES('Test Restaurant')`)
	f.repo = authrepository.NewRepository(pool)
	f.service, err = authservice.NewService(f.repo, "https://flagman.example")
	if err != nil {
		t.Fatal(err)
	}
	base := repository.NewRepository(pool, zap.NewNop())
	f.audit = auditlogservice.NewService(auditlogrepository.NewRepository(), base)
	f.settings = settingsservice.NewService(settingsrepository.NewRepository(), base, base)
	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) actor() context.Context {
	return requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: "admin"})
}

func (f *fixture) router() http.Handler {
	r := gin.New()
	r.Use(basehandler.RequestID())
	access := authhandler.NewHandler(f.service)
	r.Use(access.CORS(), basehandler.BodyLimit(1<<20))
	api := r.Group("/api/v1", access.RequireRouteAccess)
	basehandler.RegisterAuditLogRoutes(api, auditloghandler.NewHandler(f.audit))
	basehandler.RegisterSettingsRoutes(api, settingshandler.NewHandler(f.settings))
	return r
}

func request(r http.Handler, method, path, body, token, csrf, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "flagman_session", Value: token})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
