package dashboard

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
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	dashboardhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/dashboard"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	dashboardrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type dashboardFixture struct {
	ctx                     context.Context
	pool                    *pgxpool.Pool
	actor, category, supply uuid.UUID
	now                     time.Time
	service                 *Service
	token                   string
	router                  http.Handler
}

func newDashboardFixture(t *testing.T) *dashboardFixture {
	t.Helper()
	dsn := os.Getenv("FLAGMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FLAGMAN_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	if (cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1" && cfg.ConnConfig.Host != "localhost") ||
		!strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("dashboard integration tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schemaName := "dashboard_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{schemaName}.Sanitize()
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
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations: %v", err)
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
	f := &dashboardFixture{
		ctx:      ctx,
		pool:     pool,
		actor:    uuid.New(),
		category: uuid.New(),
		supply:   uuid.New(),
		now:      time.Now().UTC().Truncate(time.Microsecond),
	}
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role)VALUES($1,'Manager','dashboard@example.test','fixture','manager')`,
		f.actor,
	)
	f.exec(t, `INSERT INTO product_categories(id,name)VALUES($1,'Ingredients')`, f.category)
	f.exec(
		t,
		`INSERT INTO supplies(id,number,received_at,created_by)VALUES($1,'S14-1',$2,$3)`,
		f.supply,
		f.now.Add(-time.Hour),
		f.actor,
	)
	f.token = uuid.NewString() + uuid.NewString()
	f.exec(
		t,
		`INSERT INTO sessions(user_id,token_hash,csrf_token,expires_at)VALUES($1,$2,'csrf',clock_timestamp()+interval '12 hours')`,
		f.actor,
		authservice.SessionTokenHash(f.token),
	)
	f.service = NewService(
		dashboardrepository.NewRepository(),
		repository.NewRepository(pool, zap.NewNop()),
	)
	f.service.now = func() time.Time { return f.now }
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(basehandler.RequestID())
	basehandler.RegisterDashboardRoutes(
		router.Group("/api/v1"),
		dashboardhandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.router = router
	return f
}

func (f *dashboardFixture) get(t *testing.T, role string) dto.DashboardResponse {
	t.Helper()
	result, err := f.service.Get(f.ctx, role)
	if err != nil {
		t.Fatal(err)
	}
	if result.AttentionBatches == nil || result.HotOffers == nil || result.ProcurementRecommendations == nil ||
		result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("dashboard envelope: %+v", result)
	}
	return result
}

func (f *dashboardFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *dashboardFixture) product(t *testing.T, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(
		t,
		`INSERT INTO products(id,category_id,name,unit,display_unit)VALUES($1,$2,$3,'g','kg')`,
		id,
		f.category,
		name,
	)
	return id
}

func (f *dashboardFixture) batch(t *testing.T, product uuid.UUID, amount string, expires time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(
		t,
		`INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)
		VALUES($1,$2,$3,(SELECT COALESCE(MAX(line_number),0)+1 FROM batches WHERE supply_id=$2),GREATEST($4::numeric,0.001),$4,'explicit',$5)`,
		id,
		f.supply,
		product,
		amount,
		expires,
	)
	return id
}

type testIngredient struct {
	product uuid.UUID
	amount  string
}

func (f *dashboardFixture) dish(t *testing.T, name string, active bool, ingredients ...testIngredient) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active)VALUES($1,$2,$3)`, id, name, active)
	if len(ingredients) > 0 {
		f.recipe(t, id, 1, ingredients...)
	}
	return id
}

func (f *dashboardFixture) recipe(t *testing.T, dish uuid.UUID, version int, ingredients ...testIngredient) {
	t.Helper()
	id := uuid.New()
	f.exec(
		t,
		`INSERT INTO recipe_versions(id,dish_id,version,created_by)VALUES($1,$2,$3,$4)`,
		id,
		dish,
		version,
		f.actor,
	)
	for _, ingredient := range ingredients {
		f.exec(
			t,
			`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,$3)`,
			id,
			ingredient.product,
			ingredient.amount,
		)
	}
}

func (f *dashboardFixture) request(query string, cookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard"+query, http.NoBody).WithContext(f.ctx)
	if cookie {
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	return response
}

func (f *dashboardFixture) response(t *testing.T) dto.DashboardResponse {
	t.Helper()
	response := f.request("", true)
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard HTTP: %d %s", response.Code, response.Body.String())
	}
	var result dto.DashboardResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Header().Get("X-Request-ID") == "" || result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("dashboard metadata: %+v", result.Meta)
	}
	return result
}
