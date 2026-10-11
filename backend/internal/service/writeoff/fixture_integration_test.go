package writeoff_test

import (
	"context"
	"fmt"
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
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	writeoffhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	writeoffrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"go.uber.org/zap"
)

type fixture struct {
	movements                        *writeoffservice.HistoryService
	schema                           string
	ctx                              context.Context
	pool                             *pgxpool.Pool
	repo                             *authrepository.Repository
	service                          *authservice.Service
	user                             uuid.UUID
	writeoff                         *writeoffservice.Service
	password, hash                   string
	category, product, supply, batch uuid.UUID
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
	if (cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1" && cfg.ConnConfig.Host != "localhost") ||
		!strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("auth tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	name := "writeoff_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Test Manager','manager@example.test',$2,'admin')`,
		f.user,
		f.hash,
	)
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name) VALUES('Test Restaurant')`)
	f.repo = authrepository.NewRepository(pool)
	f.service, err = authservice.NewService(f.repo, "https://flagman.example")
	if err != nil {
		t.Fatal(err)
	}
	base := repository.NewRepository(pool, zap.NewNop())
	f.movements = writeoffservice.NewHistoryService(writeoffrepository.NewRepository(), base)
	f.writeoff = writeoffservice.NewService(writeoffrepository.NewRepository(), base, idempotency.NewService(base))
	f.category, f.product, f.supply, f.batch = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Test category')`, f.category)
	f.exec(
		t,
		`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Молоко','ml','l')`,
		f.product,
		f.category,
	)
	f.exec(
		t,
		`INSERT INTO supplies(id,number,created_by,received_at) VALUES($1,1,$2,clock_timestamp()-interval '1 hour')`,
		f.supply,
		f.user,
	)
	f.exec(
		t,
		`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,10,10,'explicit',clock_timestamp()+interval '1 day')`,
		f.batch,
		f.product,
		f.supply,
	)
	f.exec(
		t,
		`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at) VALUES($1,'receipt',10,10,'Молоко','ml',$2,clock_timestamp()-interval '1 hour')`,
		f.batch,
		f.user,
	)
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
	basehandler.RegisterStockMovementRoutes(api, writeoffhandler.NewHistoryHandler(f.movements))
	basehandler.RegisterWriteOffRoutes(api, writeoffhandler.NewHandler(f.writeoff))
	return r
}

func request(r http.Handler, method, path, body, token, csrf, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
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

func (f *fixture) body(amount string) []byte {
	return []byte(fmt.Sprintf(`{"batch_id":"%s","quantity":"%s","reason":"  Порча продукта  "}`, f.batch, amount))
}

func (f *fixture) create(t *testing.T, amount string, key uuid.UUID) model.IdempotencyResponse {
	t.Helper()
	r, err := f.writeoff.CreateIdempotent(f.actor(), f.user, key, f.body(amount))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *fixture) balance(t *testing.T) string {
	t.Helper()
	var value string
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id=$1`, f.batch).
		Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func (f *fixture) counts(t *testing.T) [4]int {
	t.Helper()
	var v [4]int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM stock_movements WHERE type='manual_writeoff'),(SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_keys)`).
		Scan(&v[0], &v[1], &v[2], &v[3]); err != nil {
		t.Fatal(err)
	}
	return v
}
