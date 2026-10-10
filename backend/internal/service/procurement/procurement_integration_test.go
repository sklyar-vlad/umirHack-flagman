package procurement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	procurementhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/procurement"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	procurementrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/procurement"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type procurementFixture struct {
	ctx                     context.Context
	pool                    *pgxpool.Pool
	service                 *Service
	router                  http.Handler
	actor, category, supply uuid.UUID
	token                   string
	now                     time.Time
}

func newProcurementFixture(t *testing.T) *procurementFixture {
	t.Helper()
	dsn := os.Getenv("FLAGMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FLAGMAN_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	if (cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1" && cfg.ConnConfig.Host != "localhost") || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("procurement integration tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schemaName := "procurement_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &procurementFixture{ctx: ctx, pool: pool, actor: uuid.New(), category: uuid.New(), supply: uuid.New(), token: uuid.NewString() + uuid.NewString(), now: time.Now().UTC().Truncate(time.Microsecond)}
	f.exec(t, `INSERT INTO users(id,name,email,password_hash,role)VALUES($1,'Operator','order-check@example.test','fixture','manager')`, f.actor)
	f.exec(t, `INSERT INTO sessions(user_id,token_hash,csrf_token,expires_at)VALUES($1,$2,'csrf',clock_timestamp()+interval '12 hours')`, f.actor, authservice.SessionTokenHash(f.token))
	f.exec(t, `INSERT INTO product_categories(id,name)VALUES($1,'Order check')`, f.category)
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by)VALUES($1,'S09',now(),$2)`, f.supply, f.actor)
	f.service = NewService(procurementrepository.NewRepository(), menurepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	f.service.now = func() time.Time { return f.now }
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(basehandler.RequestID(), basehandler.BodyLimit(1<<20))
	basehandler.RegisterProcurementRoutes(router.Group("/api/v1"), procurementhandler.NewHandler(f.service), authhandler.NewHandler(auth))
	f.router = router
	return f
}

func (f *procurementFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *procurementFixture) product(t *testing.T, name string, target *string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO products(id,category_id,name,unit,display_unit,target_stock)VALUES($1,$2,$3,'g','kg',$4)`, id, f.category, name, target)
	return id
}

func (f *procurementFixture) batch(t *testing.T, product uuid.UUID, amount string, expires time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO batches(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,(SELECT COALESCE(MAX(line_number),0)+1 FROM batches WHERE supply_id=$1),$3,$3,'explicit',$4)`, f.supply, product, amount, expires)
}

func (f *procurementFixture) request(query string, cookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/procurement-recommendations"+query, nil).WithContext(f.ctx)
	if cookie {
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	return response
}

func (f *procurementFixture) list(t *testing.T, query string) dto.ProcurementListResponse {
	t.Helper()
	response := f.request(query, true)
	if response.Code != http.StatusOK {
		t.Fatalf("procurement: %d %s", response.Code, response.Body.String())
	}
	var result dto.ProcurementListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("list envelope: %+v", result)
	}
	return result
}

func TestProcurementIntegrationTargetsAndExpiry(t *testing.T) {
	f := newProcurementFixture(t)
	target, zero, large := "1.000", "0.000", "99999999999.999"
	mixed := f.product(t, "A mixed", &target)
	f.batch(t, mixed, "0.499", f.now.Add(time.Microsecond))
	f.batch(t, mixed, "0.500", f.now.Add(time.Hour))
	f.batch(t, mixed, "50.000", f.now)
	expired := f.product(t, "B expired", &target)
	f.batch(t, expired, "50.000", f.now.Add(-time.Microsecond))
	empty := f.product(t, "C empty", &target)
	largeExpired := f.product(t, "D large expired", &target)
	f.batch(t, largeExpired, large, f.now)
	f.batch(t, largeExpired, large, f.now)
	f.product(t, "No target", nil)
	f.product(t, "Zero target", &zero)
	atTarget := f.product(t, "At target", &target)
	f.batch(t, atTarget, "1.000", f.now.Add(time.Hour))
	above := f.product(t, "Above", &large)
	f.batch(t, above, large, f.now.Add(time.Hour))
	f.batch(t, above, large, f.now.Add(time.Hour))
	result := f.list(t, "")
	if result.Total != 4 {
		t.Fatalf("unexpected total: %+v", result)
	}
	byID := map[uuid.UUID]dto.ProcurementItem{}
	for _, item := range result.Items {
		byID[item.ProductID] = item
	}
	if byID[mixed].RecommendedQuantity != "0.001" || byID[mixed].AvailableQuantity != "0.999" || byID[mixed].ExpiredQuantity != "50.000" || byID[expired].RecommendedQuantity != "1.000" || byID[empty].RecommendedQuantity != "1.000" {
		t.Fatalf("stock calculation: %+v", result.Items)
	}
	if byID[largeExpired].ExpiredQuantity != "199999999999.998" || byID[largeExpired].RecommendedQuantity != "1.000" {
		t.Fatalf("expired aggregate truncated: %+v", byID[largeExpired])
	}
	for _, table := range []string{"orders", "order_attempts", "stock_movements", "audit_log", "outbox_events"} {
		var count int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("read wrote %s: %d", table, count)
		}
	}
	result = f.list(t, "?page=5&page_size=1")
	if result.Total != 4 || len(result.Items) != 0 {
		t.Fatalf("empty page total: %+v", result)
	}
}

func (f *procurementFixture) dish(t *testing.T, name string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active)VALUES($1,$2,$3)`, id, name, active)
	return id
}
func (f *procurementFixture) recipe(t *testing.T, dish uuid.UUID, version int, ingredients map[uuid.UUID]string) {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by)VALUES($1,$2,$3,$4)`, id, dish, version, f.actor)
	for product, amount := range ingredients {
		f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,$3)`, id, product, amount)
	}
}

func TestProcurementIntegrationReasonsAndCurrentShortages(t *testing.T) {
	f := newProcurementFixture(t)
	target := "10.000"
	x := f.product(t, "Mixed", &target)
	y := f.product(t, "Expired", &target)
	z := f.product(t, "Empty", &target)
	f.batch(t, x, "0.500", f.now.Add(time.Hour))
	f.batch(t, x, "50.000", f.now)
	f.batch(t, y, "50.000", f.now)
	blocked := f.dish(t, "A blocked", true)
	f.recipe(t, blocked, 1, map[uuid.UUID]string{x: "0.501", y: "1.000"})
	sufficient := f.dish(t, "B sufficient", true)
	f.recipe(t, sufficient, 1, map[uuid.UUID]string{x: "0.500", z: "1.000"})
	available := f.dish(t, "Available", true)
	f.recipe(t, available, 1, map[uuid.UUID]string{x: "0.500"})
	inactive := f.dish(t, "Inactive", false)
	f.recipe(t, inactive, 1, map[uuid.UUID]string{x: "1.000", y: "1.000"})
	missing := f.dish(t, "Missing recipe", true)
	_ = missing
	latest := f.dish(t, "C latest", true)
	f.recipe(t, latest, 1, map[uuid.UUID]string{x: "1.000"})
	f.recipe(t, latest, 2, map[uuid.UUID]string{y: "1.000"})
	result := f.list(t, "")
	if result.Total != 3 {
		t.Fatalf("total: %+v", result)
	}
	byID := map[uuid.UUID]dto.ProcurementItem{}
	for _, item := range result.Items {
		byID[item.ProductID] = item
		if item.Reason == "" || item.AffectedDishes == nil {
			t.Fatalf("missing explanation: %+v", item)
		}
	}
	if !reflect.DeepEqual(byID[x].ReasonCodes, []string{"below_target"}) || !reflect.DeepEqual(byID[y].ReasonCodes, []string{"below_target", "no_valid_stock", "only_expired_stock"}) || !reflect.DeepEqual(byID[z].ReasonCodes, []string{"below_target", "no_valid_stock"}) {
		t.Fatalf("reason codes: %+v", result.Items)
	}
	if !reflect.DeepEqual(byID[x].AffectedDishes, []dto.AffectedDish{{ID: blocked, Name: "A blocked"}}) || !reflect.DeepEqual(byID[y].AffectedDishes, []dto.AffectedDish{{ID: blocked, Name: "A blocked"}, {ID: latest, Name: "C latest"}}) || !reflect.DeepEqual(byID[z].AffectedDishes, []dto.AffectedDish{{ID: sufficient, Name: "B sufficient"}}) {
		t.Fatalf("current shortage links: %+v", result.Items)
	}
	f.now = f.now.Add(time.Hour)
	after := f.list(t, "")
	for _, item := range after.Items {
		if item.ProductID == x && (item.RecommendedQuantity != "10.000" || len(item.ReasonCodes) != 3 || len(item.AffectedDishes) != 3) {
			t.Fatalf("expiry recalculation: %+v", item)
		}
	}
}

func TestProcurementIntegrationCurrentPermissionsAndSessions(t *testing.T) {
	f := newProcurementFixture(t)
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		response := f.request("", true)
		want := http.StatusOK
		if role == "order_operator" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("%s: %d %s", role, response.Code, response.Body.String())
		}
	}
	if response := f.request("?page=0", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing session: %d", response.Code)
	}
	f.exec(t, `UPDATE users SET role='manager',is_active=false WHERE id=$1`, f.actor)
	if response := f.request("", true); response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user: %d", response.Code)
	}
	f.exec(t, `UPDATE users SET is_active=true WHERE id=$1`, f.actor)
	f.exec(t, `UPDATE sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour'`)
	if response := f.request("?page=0", true); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: %d", response.Code)
	}
}
