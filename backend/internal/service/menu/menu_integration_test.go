package menu

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
	menuhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type menuFixture struct {
	ctx                     context.Context
	pool                    *pgxpool.Pool
	service                 *Service
	router                  http.Handler
	actor, category, supply uuid.UUID
	token                   string
	now                     time.Time
}

func newMenuFixture(t *testing.T) *menuFixture {
	t.Helper()
	dsn := os.Getenv("MENU_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MENU_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := pgx.Identifier{"menu_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
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
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = strings.Trim(schema, `"`)
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
	f := &menuFixture{
		ctx:      ctx,
		pool:     pool,
		actor:    uuid.New(),
		category: uuid.New(),
		supply:   uuid.New(),
		token:    uuid.NewString() + uuid.NewString(),
		now:      time.Now().UTC().Truncate(time.Microsecond),
	}
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Operator','menu@example.test','fixture','order_operator')`,
		f.actor,
	)
	f.exec(
		t,
		`INSERT INTO sessions(user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,'csrf',clock_timestamp()+interval '12 hours')`,
		f.actor,
		authservice.SessionTokenHash(f.token),
	)
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Menu')`, f.category)
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by) VALUES($1,'S08',now(),$2)`, f.supply, f.actor)
	f.service = NewService(menurepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	f.service.now = func() time.Time { return f.now }
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(basehandler.RequestID())
	basehandler.RegisterMenuRoutes(
		router.Group("/api/v1"),
		menuhandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.router = router
	return f
}

func (f *menuFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *menuFixture) product(t *testing.T, name string) uuid.UUID {
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

func (f *menuFixture) batch(t *testing.T, product uuid.UUID, quantity string, expires time.Time) {
	t.Helper()
	f.exec(
		t,
		`INSERT INTO batches(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,(SELECT COALESCE(MAX(line_number),0)+1 FROM batches WHERE supply_id=$1),$3,$3,'explicit',$4)`,
		f.supply,
		product,
		quantity,
		expires,
	)
}

func (f *menuFixture) dish(t *testing.T, name string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active)VALUES($1,$2,$3)`, id, name, active)
	return id
}

func (f *menuFixture) recipe(t *testing.T, dish uuid.UUID, version int, product uuid.UUID, quantity string) {
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
	f.exec(
		t,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,$3)`,
		id,
		product,
		quantity,
	)
}

func (f *menuFixture) request(endpoint string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1"+endpoint, http.NoBody)
	if authenticated {
		request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}

func (f *menuFixture) list(t *testing.T, endpoint string) dto.DishListResponse {
	t.Helper()
	response := f.request(endpoint, true)
	if response.Code != 200 {
		t.Fatalf("%s: %d %s", endpoint, response.Code, response.Body.String())
	}
	var result dto.DishListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMenuIntegrationLatestRecipeAndFilters(t *testing.T) {
	f := newMenuFixture(t)
	cheese := f.product(t, "Cheese")
	chicken := f.product(t, "Chicken")
	cream := f.product(t, "Cream")
	f.batch(t, cheese, "1000.000", f.now.Add(time.Hour))
	f.batch(t, cream, "500.000", f.now)
	available := f.dish(t, "A available", true)
	missing := f.dish(t, "B chicken", true)
	expired := f.dish(t, "C cream", true)
	empty := f.dish(t, "D no recipe", true)
	inactive := f.dish(t, "E inactive", false)

	f.recipe(t, available, 2, cheese, "300.000")
	f.recipe(t, available, 1, chicken, "100.000")
	f.recipe(t, missing, 1, chicken, "100.000")
	f.recipe(t, expired, 1, cream, "100.000")
	f.recipe(t, inactive, 1, cheese, "100.000")
	all := f.list(t, "/dishes")
	if all.Total != 4 || len(all.Items) != 4 {
		t.Fatalf("active list: %+v", all)
	}
	if all.Items[0].ID != available || all.Items[0].AvailablePortions != "3" || *all.Items[0].RecipeVersion != 2 ||
		all.Items[0].Recipe[0].ProductID != cheese {
		t.Fatalf("latest recipe: %+v", all.Items[0])
	}
	if all.Items[1].Shortages[0].ReasonCode != "ingredient_shortage" ||
		all.Items[2].Shortages[0].ReasonCode != "only_expired_stock" ||
		all.Items[3].UnavailabilityReasons[0] != "recipe_missing" {
		t.Fatalf("shortages: %+v", all.Items)
	}
	if all.Items[3].ID != empty || all.Items[3].RecipeVersion != nil || all.Items[3].Recipe == nil ||
		all.Items[3].Shortages == nil {
		t.Fatal("missing recipe representation")
	}
	page := f.list(t, "/dishes?availability=unavailable&page=2&page_size=1")
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].ID != expired {
		t.Fatalf("filtered pagination: %+v", page)
	}
	if result := f.list(t, "/dishes?search=chicken"); result.Total != 1 || result.Items[0].ID != missing {
		t.Fatalf("search: %+v", result)
	}
	if result := f.list(
		t,
		"/dishes?is_active=false",
	); result.Total != 1 || result.Items[0].ID != inactive ||
		result.Items[0].UnavailabilityReasons[0] != "dish_inactive" {
		t.Fatalf("inactive: %+v", result)
	}
	if result := f.list(t, "/dishes?is_active=all"); result.Total != 5 {
		t.Fatalf("all: %+v", result)
	}
	if result := f.list(t, "/dishes?page=100"); result.Total != 4 || len(result.Items) != 0 || result.Items == nil {
		t.Fatalf("empty page: %+v", result)
	}
	if result := f.list(t, "/dishes?search=%25"); result.Total != 0 {
		t.Fatal("percent was interpreted as wildcard")
	}
	detail := f.request("/dishes/"+available.String(), true)
	if detail.Code != 200 {
		t.Fatal(detail.Body.String())
	}
	var dish dto.Dish
	if err := json.Unmarshal(detail.Body.Bytes(), &dish); err != nil {
		t.Fatal(err)
	}
	if dish.AvailablePortions != "3" || dish.ID != available {
		t.Fatalf("details: %+v", dish)
	}
	for endpoint, status := range map[string]int{"/dishes/" + uuid.NewString(): 404, "/dishes/not-a-uuid": 422, "/dishes?availability=invalid": 422, "/stop-list?is_active=all": 422} {
		response := f.request(endpoint, true)
		if response.Code != status {
			t.Errorf("%s: %d want %d", endpoint, response.Code, status)
		}
	}
	for _, endpoint := range []string{"/dishes", "/stop-list", "/dishes/" + available.String()} {
		if response := f.request(endpoint, false); response.Code != 401 {
			t.Errorf("anonymous %s: %d", endpoint, response.Code)
		}
	}
	for _, role := range []string{"storekeeper", "admin", "manager", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$1 WHERE id=$2`, role, f.actor)
		for _, endpoint := range []string{"/dishes", "/stop-list", "/dishes/" + available.String()} {
			want := 200
			if role == "storekeeper" {
				want = 403
			}
			if response := f.request(endpoint, true); response.Code != want {
				t.Errorf("role %s %s: %d want %d", role, endpoint, response.Code, want)
			}
		}
	}
}

func TestMenuIntegrationStopListRecalculatesOnReceiptAndExpiry(t *testing.T) {
	f := newMenuFixture(t)
	product := f.product(t, "Cream")
	active := f.dish(t, "Active", true)
	inactive := f.dish(t, "Inactive", false)
	f.recipe(t, active, 1, product, "100.000")
	f.recipe(t, inactive, 1, product, "100.000")
	initial := f.list(t, "/stop-list")
	if initial.Total != 1 || initial.Items[0].ID != active ||
		initial.Items[0].Shortages[0].ReasonCode != "ingredient_shortage" {
		t.Fatalf("initial: %+v", initial)
	}

	f.batch(t, product, "300.000", f.now.Add(time.Hour))
	if supplied := f.list(t, "/stop-list"); supplied.Total != 0 || supplied.Items == nil {
		t.Fatalf("after receipt: %+v", supplied)
	}
	if dishes := f.list(
		t,
		"/dishes?availability=available",
	); dishes.Total != 1 ||
		dishes.Items[0].AvailablePortions != "3" {
		t.Fatalf("after receipt menu: %+v", dishes)
	}

	f.now = f.now.Add(time.Hour)
	expired := f.list(t, "/stop-list")
	if expired.Total != 1 || expired.Items[0].ID != active ||
		expired.Items[0].Shortages[0].ReasonCode != "only_expired_stock" ||
		expired.Items[0].Shortages[0].AvailableQuantity != "0.000" {
		t.Fatalf("expiry boundary: %+v", expired)
	}
	if expired.Meta["server_time"] != f.now.Format(time.RFC3339Nano) ||
		expired.Items[0].Meta["server_time"] != expired.Meta["server_time"] {
		t.Fatal("inconsistent calculation time")
	}

	f.batch(t, product, "50.000", f.now.Add(time.Hour))
	if mixed := f.list(
		t,
		"/stop-list",
	); mixed.Items[0].Shortages[0].ReasonCode != "ingredient_shortage" ||
		mixed.Items[0].Shortages[0].MissingQuantity != "50.000" {
		t.Fatalf("mixed: %+v", mixed)
	}
}
