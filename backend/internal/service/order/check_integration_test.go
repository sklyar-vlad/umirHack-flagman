package order

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
	orderhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/order"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type orderFixture struct {
	ctx                     context.Context
	pool                    *pgxpool.Pool
	service                 *Service
	router                  http.Handler
	actor, category, supply uuid.UUID
	token                   string
	now                     time.Time
}

func newOrderFixture(t *testing.T) *orderFixture {
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
		t.Fatal("order integration tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schemaName := "order_check_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &orderFixture{
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
		`INSERT INTO users(id,name,email,password_hash,role)VALUES($1,'Operator','order-check@example.test','fixture','order_operator')`,
		f.actor,
	)
	f.exec(
		t,
		`INSERT INTO sessions(user_id,token_hash,csrf_token,expires_at)VALUES($1,$2,'csrf',clock_timestamp()+interval '12 hours')`,
		f.actor,
		authservice.SessionTokenHash(f.token),
	)
	f.exec(t, `INSERT INTO product_categories(id,name)VALUES($1,'Order check')`, f.category)
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by)VALUES($1,'S09',now(),$2)`, f.supply, f.actor)
	f.service = NewService(menurepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	f.service.now = func() time.Time { return f.now }
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(basehandler.RequestID(), basehandler.BodyLimit(1<<20))
	basehandler.RegisterOrderCheckRoutes(
		router.Group("/api/v1"),
		orderhandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.router = router
	return f
}

func (f *orderFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *orderFixture) product(t *testing.T, name string) uuid.UUID {
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

func (f *orderFixture) batch(t *testing.T, product uuid.UUID, quantity string, expires time.Time) {
	t.Helper()
	f.exec(
		t,
		`INSERT INTO batches(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)VALUES($1,$2,(SELECT COALESCE(MAX(line_number),0)+1 FROM batches WHERE supply_id=$1),$3,$3,'explicit',$4)`,
		f.supply,
		product,
		quantity,
		expires,
	)
}

func (f *orderFixture) dish(t *testing.T, name string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active)VALUES($1,$2,$3)`, id, name, active)
	return id
}

func (f *orderFixture) recipe(t *testing.T, dish uuid.UUID, version int, ingredients map[uuid.UUID]string) {
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
	for product, quantity := range ingredients {
		f.exec(
			t,
			`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,$3)`,
			id,
			product,
			quantity,
		)
	}
}

func (f *orderFixture) request(body string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders/check", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authenticated {
		request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}

func (f *orderFixture) check(t *testing.T, items ...dto.OrderRequestItem) dto.OrderCheck {
	t.Helper()
	body, err := json.Marshal(dto.OrderRequest{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	response := f.request(string(body), true)
	if response.Code != http.StatusOK {
		t.Fatalf("check: %d %s", response.Code, response.Body.String())
	}
	var result dto.OrderCheck
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || result.Requirements == nil || result.Shortages == nil || result.Issues == nil ||
		result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("invalid response arrays/time: %s", response.Body.String())
	}
	return result
}

func (f *orderFixture) snapshot(t *testing.T) string {
	t.Helper()
	var result string
	if err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'batches',COALESCE((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM batches b),'[]'::jsonb),
		'orders',(SELECT count(*) FROM orders), 'items',(SELECT count(*) FROM order_items),
		'movements',(SELECT count(*) FROM stock_movements), 'attempts',(SELECT count(*) FROM order_attempts),
		'audit',(SELECT count(*) FROM audit_log), 'outbox',(SELECT count(*) FROM outbox_events),
		'idempotency',(SELECT count(*) FROM idempotency_keys))::text`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOrderCheckIntegrationSharedDeficitAndNoWrites(t *testing.T) {
	f := newOrderFixture(t)
	product := f.product(t, "Cheese")
	f.batch(t, product, "1.000", f.now.Add(time.Hour))
	first, second := f.dish(t, "Z pizza", true), f.dish(t, "A pizza", true)
	f.recipe(t, first, 1, map[uuid.UUID]string{product: "0.300"})
	f.recipe(t, second, 1, map[uuid.UUID]string{product: "0.300"})
	before := f.snapshot(t)
	for _, id := range []uuid.UUID{first, second} {
		if !f.check(t, dto.OrderRequestItem{DishID: id.String(), Portions: 2}).CanCreate {
			t.Fatal("dish individually should be available")
		}
	}
	for range 2 {
		result := f.check(
			t,
			dto.OrderRequestItem{DishID: first.String(), Portions: 1},
			dto.OrderRequestItem{DishID: second.String(), Portions: 2},
			dto.OrderRequestItem{DishID: strings.ToUpper(first.String()), Portions: 1},
		)
		if result.CanCreate || len(result.Requirements) != 1 || len(result.Shortages) != 1 || len(result.Issues) != 0 ||
			result.Items[0].DishID != first ||
			result.Items[0].Portions != 2 ||
			result.Items[1].DishID != second {
			t.Fatalf("shared basket: %+v", result)
		}
		requirement := result.Requirements[0]
		if requirement.RequiredQuantity != "1.200" || requirement.AvailableQuantity != "1.000" ||
			requirement.MissingQuantity != "0.200" ||
			result.Shortages[0].ReasonCode != "ingredient_shortage" {
			t.Fatalf("wrong aggregate: %+v", result)
		}
	}
	for _, body := range []string{`{"items":[]}`, `{"items":[{"dish_id":"` + uuid.NewString() + `","portions":1}]}`} {
		if response := f.request(body, true); response.Code != 422 {
			t.Fatalf("validation: %d %s", response.Code, response.Body.String())
		}
	}
	if after := f.snapshot(t); after != before {
		t.Fatalf("check wrote data:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestOrderCheckIntegrationStockExpiryAndLargeSums(t *testing.T) {
	f := newOrderFixture(t)
	atBoundary, mixed, sufficient, large := f.product(
		t,
		"Boundary",
	), f.product(
		t,
		"Mixed",
	), f.product(
		t,
		"Sufficient",
	), f.product(
		t,
		"Large",
	)
	f.batch(t, atBoundary, "10.000", f.now)
	f.batch(t, mixed, "0.500", f.now.Add(time.Hour))
	f.batch(t, mixed, "10.000", f.now.Add(-time.Second))
	f.batch(t, sufficient, "1.000", f.now.Add(time.Microsecond))
	f.batch(t, large, "99999999999.999", f.now.Add(time.Hour))
	f.batch(t, large, "99999999999.999", f.now.Add(time.Hour))
	dish := f.dish(t, "All products", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{atBoundary: "1.000", mixed: "1.000", sufficient: "1.000", large: "0.001"})
	before := f.snapshot(t)
	result := f.check(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	if result.CanCreate || len(result.Requirements) != 4 || len(result.Shortages) != 2 {
		t.Fatalf("stock response: %+v", result)
	}
	byID := make(map[uuid.UUID]dto.Requirement)
	for _, requirement := range result.Requirements {
		byID[requirement.ProductID] = requirement
	}
	if byID[atBoundary].AvailableQuantity != "0.000" || byID[sufficient].MissingQuantity != "0.000" ||
		byID[large].AvailableQuantity != "199999999999.998" ||
		byID[large].RequiredQuantity != "0.001" {
		t.Fatalf("boundary/large stock: %+v", result.Requirements)
	}
	if result.Shortages[0].ProductID != atBoundary || result.Shortages[0].ReasonCode != "only_expired_stock" ||
		result.Shortages[0].ExpiredQuantity != "10.000" ||
		result.Shortages[1].ProductID != mixed ||
		result.Shortages[1].ReasonCode != "ingredient_shortage" ||
		result.Shortages[1].MissingQuantity != "0.500" {
		t.Fatalf("expiry reasons: %+v", result.Shortages)
	}
	if f.snapshot(t) != before {
		t.Fatal("expiry/shortage check changed storage")
	}
}

type snapshotProbe struct {
	t       *testing.T
	fixture *orderFixture
	dish    uuid.UUID
	product uuid.UUID
}

func (p snapshotProbe) Load(
	ctx context.Context,
	q repository.DBTX,
	filter *model.DishFilter,
	now time.Time,
) ([]model.Dish, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	var readOnly, isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return nil, err
	}
	if readOnly != "on" || isolation != "repeatable read" {
		p.t.Fatalf("transaction mode: %s %s", readOnly, isolation)
	}
	repo := menurepository.NewRepository()
	before, err := repo.Load(ctx, q, filter, now)
	if err != nil {
		return nil, err
	}
	// Another connection commits a recipe and stock change while the check's
	// read-only transaction remains open. Its snapshot must not change.
	p.fixture.recipe(p.t, p.dish, 2, map[uuid.UUID]string{p.product: "2.000"})
	p.fixture.exec(p.t, `UPDATE batches SET remaining_quantity='0.000' WHERE product_id=$1`, p.product)
	after, err := repo.Load(ctx, q, filter, now)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(before, after) {
		p.t.Fatalf("snapshot changed during check: before=%+v after=%+v", before, after)
	}
	return after, nil
}

func TestOrderCheckIntegrationRepeatableRead(t *testing.T) {
	f := newOrderFixture(t)
	product := f.product(t, "Cheese")
	f.batch(t, product, "1.000", f.now.Add(time.Hour))
	dish := f.dish(t, "Pizza", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	service := NewService(
		snapshotProbe{t: t, fixture: f, dish: dish, product: product},
		repository.NewRepository(f.pool, zap.NewNop()),
	)
	service.now = f.service.now
	request := dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 1}}}
	first, err := service.Check(f.ctx, request)
	if err != nil || !first.CanCreate || *first.Items[0].RecipeVersion != 1 ||
		first.Requirements[0].AvailableQuantity != "1.000" {
		t.Fatalf("original snapshot: %+v %v", first, err)
	}
	second := f.check(t, request.Items...)
	if second.CanCreate || *second.Items[0].RecipeVersion != 2 || second.Requirements[0].RequiredQuantity != "2.000" ||
		second.Requirements[0].AvailableQuantity != "0.000" {
		t.Fatalf("next check did not see committed changes: %+v", second)
	}
}

func TestOrderCheckIntegrationRecipeChangesAndUnavailableDishes(t *testing.T) {
	f := newOrderFixture(t)
	oldProduct, currentProduct := f.product(t, "Old product"), f.product(t, "Current product")
	f.batch(t, oldProduct, "100.000", f.now.Add(time.Hour))
	f.batch(t, currentProduct, "1.000", f.now.Add(time.Hour))
	dish := f.dish(t, "Pizza", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{oldProduct: "10.000"})
	initial := f.check(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 2})
	if !initial.CanCreate || *initial.Items[0].RecipeVersion != 1 {
		t.Fatalf("initial recipe: %+v", initial)
	}
	f.recipe(t, dish, 2, map[uuid.UUID]string{currentProduct: "0.300"})
	before := f.snapshot(t)
	stale := f.check(
		t,
		dto.OrderRequestItem{DishID: dish.String(), Portions: 2, RecipeVersion: initial.Items[0].RecipeVersion},
	)
	if stale.CanCreate || len(stale.Issues) != 1 || stale.Issues[0].Code != "recipe_changed" ||
		*stale.Issues[0].CurrentRecipeVersion != 2 ||
		*stale.Items[0].RecipeVersion != 2 ||
		len(stale.Requirements) != 1 ||
		stale.Requirements[0].ProductID != currentProduct ||
		stale.Requirements[0].RequiredQuantity != "0.600" {
		t.Fatalf("stale version did not use latest recipe: %+v", stale)
	}
	if f.snapshot(t) != before {
		t.Fatal("recipe issue check wrote storage")
	}
	if current := f.check(
		t,
		dto.OrderRequestItem{DishID: dish.String(), Portions: 2},
	); !current.CanCreate ||
		*current.Items[0].RecipeVersion != 2 {
		t.Fatalf("omitted version should accept latest recipe: %+v", current)
	}
	f.exec(t, `UPDATE dishes SET is_active=false WHERE id=$1`, dish)
	before = f.snapshot(t)
	inactive := f.check(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 2})
	if inactive.CanCreate || len(inactive.Issues) != 1 || inactive.Issues[0].Code != "dish_inactive" ||
		len(inactive.Shortages) != 0 ||
		inactive.Requirements[0].RequiredQuantity != "0.600" {
		t.Fatalf("inactive dish: %+v", inactive)
	}
	if f.snapshot(t) != before {
		t.Fatal("inactive dish check wrote storage")
	}
	missing := f.dish(t, "No recipe", true)
	empty := f.dish(t, "Empty latest recipe", true)
	f.recipe(t, empty, 1, map[uuid.UUID]string{oldProduct: "1.000"})
	f.recipe(t, empty, 2, nil)
	before = f.snapshot(t)
	for _, id := range []uuid.UUID{missing, empty} {
		result := f.check(t, dto.OrderRequestItem{DishID: id.String(), Portions: 1})
		if result.CanCreate || len(result.Issues) != 1 || result.Issues[0].Code != "recipe_missing" ||
			len(result.Requirements) != 0 ||
			len(result.Shortages) != 0 {
			t.Fatalf("missing/empty recipe: %+v", result)
		}
	}
	if f.snapshot(t) != before {
		t.Fatal("missing recipe check wrote storage")
	}
}

func TestOrderCheckIntegrationAccessAndInvalidInput(t *testing.T) {
	f := newOrderFixture(t)
	product := f.product(t, "Cheese")
	f.batch(t, product, "1.000", f.now.Add(time.Hour))
	dish := f.dish(t, "Pizza", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "0.300"})
	body := `{"items":[{"dish_id":"` + dish.String() + `","portions":1}]}`
	before := f.snapshot(t)
	if response := f.request("{", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated check: %d %s", response.Code, response.Body.String())
	}
	for _, role := range []string{"admin", "manager", "order_operator", "storekeeper"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		response := f.request(body, true)
		want := http.StatusOK
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("role %s: %d %s", role, response.Code, response.Body.String())
		}
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	for _, tc := range []struct {
		body string
		code int
	}{
		{"{", 400},
		{`[]`, 422},
		{`{"items":[]}`, 422},
		{strings.Replace(body, `"portions":1`, `"portions":1.5`, 1), 422},
		{strings.Replace(body, `"portions":1`, `"portions":0`, 1), 422},
		{strings.Replace(body, `"portions":1`, `"portions":1001`, 1), 422},
		{`{"items":[{"dish_id":"` + dish.String() + `","portions":1000},{"dish_id":"` + dish.String() + `","portions":1}]}`, 422},
		{strings.Replace(body, dish.String(), uuid.NewString(), 1), 422},
	} {
		response := f.request(tc.body, true)
		if response.Code != tc.code {
			t.Fatalf("body %s: %d %s", tc.body, response.Code, response.Body.String())
		}
		var result dto.ErrorResponse
		if err := json.Unmarshal(
			response.Body.Bytes(),
			&result,
		); err != nil ||
			result.RequestID != response.Header().Get("X-Request-ID") {
			t.Fatalf("error request ID: %s %v", response.Body.String(), err)
		}
	}
	f.exec(t, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE user_id=$1`, f.actor)
	if response := f.request(body, true); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session: %d %s", response.Code, response.Body.String())
	}
	if f.snapshot(t) != before {
		t.Fatal("authorization or validation wrote order/stock state")
	}
}
