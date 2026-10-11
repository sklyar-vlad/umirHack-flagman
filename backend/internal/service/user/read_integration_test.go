package user_test

import (
	"context"
	"encoding/json"
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
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	userhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/user"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	userrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/user"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	userservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/user"
	"go.uber.org/zap"
)

type fixture struct {
	ctx            context.Context
	pool           *pgxpool.Pool
	repo           *authrepository.Repository
	service        *authservice.Service
	user           uuid.UUID
	users          *userservice.Service
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
	name := "users_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &fixture{ctx: ctx, pool: pool, user: uuid.New(), password: "correct test password"}
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
	f.users = userservice.NewService(userrepository.NewRepository(), base, base)
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
	basehandler.RegisterUserReadRoutes(api, userhandler.NewHandler(f.users))
	basehandler.RegisterUserWriteRoutes(api, userhandler.NewHandler(f.users))
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

func TestUserReadsSearchFiltersPaginationAndSafeDetails(t *testing.T) {
	f := newFixture(t)
	ctx := f.actor()
	a, b := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO users(id,name,email,password_hash,role,is_active) VALUES($1,'Same_%','literal@example.test',$3,'manager',false),($2,'Same_%','other@example.test',$3,'storekeeper',true)`, a, b, f.hash)
	items, total, err := f.users.List(ctx, model.UserFilter{Page: 1, PageSize: 1})
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("list %v %d %v", items, total, err)
	}
	items, total, err = f.users.List(ctx, model.UserFilter{Page: 99, PageSize: 1})
	if err != nil || total != 3 || items == nil || len(items) != 0 {
		t.Fatal("empty page lost total/array")
	}
	search := "_%"
	active := false
	role := "manager"
	items, total, err = f.users.List(ctx, model.UserFilter{Page: 1, PageSize: 50, Search: &search, IsActive: &active, Role: &role})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != a {
		t.Fatal("literal/combined filters failed", err)
	}
	search = "OTHER@EXAMPLE"
	items, total, err = f.users.List(ctx, model.UserFilter{Page: 1, PageSize: 50, Search: &search})
	if err != nil || total != 1 || items[0].ID != b {
		t.Fatal("email search failed", err)
	}
	search = "\\"
	items, total, err = f.users.List(ctx, model.UserFilter{Page: 1, PageSize: 50, Search: &search})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("backslash search failed", err)
	}
	u, err := f.users.Get(ctx, a)
	if err != nil || u.ID != a || u.IsActive {
		t.Fatal("details", err)
	}
	raw, _ := json.Marshal(u)
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), f.hash) {
		t.Fatal("secret in DTO")
	}
	if _, err = f.users.Get(ctx, uuid.New()); err != apperrors.ErrNotFound {
		t.Fatal("missing user", err)
	}
	roles, err := f.users.Roles(ctx)
	if err != nil || len(roles) != 4 {
		t.Fatal("roles", err)
	}
	// ID tie-breaking produces the same order across repeated pages.
	search = "Same_%"
	items, _, err = f.users.List(ctx, model.UserFilter{Page: 1, PageSize: 50, Search: &search})
	if err != nil || len(items) != 2 || items[0].ID.String() > items[1].ID.String() {
		t.Fatal("unstable order")
	}
}

func TestAdminReadHTTPPolicyAndInput(t *testing.T) {
	f := newFixture(t)
	r := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/roles", "/users", "/users/" + f.user.String()} {
			expected := 403
			if role == "admin" {
				expected = 200
			}
			w := request(r, "GET", path, "", token, "", "")
			if w.Code != expected {
				t.Fatalf("%s %s: %d %s", role, path, w.Code, w.Body)
			}
			if w := request(r, "GET", path, "", "", "", ""); w.Code != 401 {
				t.Fatal("public user read")
			}
		}
		if role == "admin" {
			for _, path := range []string{"/users?role=no", "/users?role=admin&role=manager", "/users?page_size=101", "/users?search=x&search=y", "/users?is_active=no", "/users?sort=email", "/users?unknown=1", "/users/bad-id"} {
				if w := request(r, "GET", path, "", token, "", ""); w.Code != 422 {
					t.Fatalf("invalid query %s: %d", path, w.Code)
				}
			}
			w := request(r, "GET", "/users", "", token, "", "")
			var list dto.UserListResponse
			if json.Unmarshal(w.Body.Bytes(), &list) != nil || list.Total != 1 || len(list.Items) != 1 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("list contract")
			}
		}
	}
}
