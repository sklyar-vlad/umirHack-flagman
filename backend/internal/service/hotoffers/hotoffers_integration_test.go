package hotoffers

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
	offershandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/hotoffers"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	offersrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type offersFixture struct {
	ctx                     context.Context
	pool                    *pgxpool.Pool
	actor, category, supply uuid.UUID
	now                     time.Time
	service                 *Service
	router                  http.Handler
	token                   string
}

func newOffersFixture(t *testing.T) *offersFixture {
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
		t.Fatal("hot offers integration tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schemaName := "hot_offers_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &offersFixture{
		ctx:      ctx,
		pool:     pool,
		actor:    uuid.New(),
		category: uuid.New(),
		supply:   uuid.New(),
		now:      time.Now().UTC().Truncate(time.Microsecond),
	}
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role)VALUES($1,'Manager','offers@example.test','fixture','manager')`,
		f.actor,
	)
	f.exec(t, `INSERT INTO product_categories(id,name)VALUES($1,'Ingredients')`, f.category)
	f.exec(
		t,
		`INSERT INTO supplies(id,number,received_at,created_by)VALUES($1,'S13-1',$2,$3)`,
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
		offersrepository.NewRepository(),
		menurepository.NewRepository(),
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
	basehandler.RegisterHotOffersRoutes(
		router.Group("/api/v1"),
		offershandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.router = router
	return f
}

func (f *offersFixture) request(query string, cookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hot-offers"+query, http.NoBody).WithContext(f.ctx)
	if cookie {
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	return response
}

func (f *offersFixture) list(t *testing.T, query string) dto.HotOffersListResponse {
	t.Helper()
	response := f.request(query, true)
	if response.Code != http.StatusOK {
		t.Fatalf("hot offers: %d %s", response.Code, response.Body.String())
	}
	var result dto.HotOffersListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("list envelope: %+v", result)
	}
	return result
}

func (f *offersFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *offersFixture) product(t *testing.T, name string) uuid.UUID {
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

func (f *offersFixture) batch(t *testing.T, product uuid.UUID, amount string, expires time.Time) uuid.UUID {
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

func (f *offersFixture) dish(t *testing.T, name string, active bool, ingredients ...testIngredient) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active)VALUES($1,$2,$3)`, id, name, active)
	if len(ingredients) > 0 {
		f.recipe(t, id, 1, ingredients...)
	}
	return id
}

func (f *offersFixture) recipe(t *testing.T, dish uuid.UUID, version int, ingredients ...testIngredient) {
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

func (f *offersFixture) simulation(t *testing.T) ([]simulatedOffer, int) {
	t.Helper()
	var offers []simulatedOffer
	var hours int
	err := repository.NewRepository(f.pool, zap.NewNop()).
		WithinTx(f.ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
			active := true
			dishes, err := menurepository.NewRepository().Load(f.ctx, tx, &model.DishFilter{IsActive: &active}, f.now)
			if err != nil {
				return err
			}
			products := []uuid.UUID{}
			seen := map[uuid.UUID]bool{}
			for _, d := range dishes {
				for _, i := range d.Recipe {
					if !seen[i.ProductID] {
						products = append(products, i.ProductID)
						seen[i.ProductID] = true
					}
				}
			}
			batches, threshold, err := offersrepository.NewRepository().Load(f.ctx, tx, products, f.now)
			if err != nil {
				return err
			}
			hours = threshold
			offers, err = simulate(dishes, batches, f.now, hours)
			return err
		})
	if err != nil {
		t.Fatal(err)
	}
	return offers, hours
}

func TestOffersIntegrationSimulation(t *testing.T) {
	f := newOffersFixture(t)
	product := f.product(t, "Моцарелла")
	missing := f.product(t, "Missing")
	old := f.batch(t, product, "50.000", f.now)
	first := f.batch(t, product, "0.250", f.now.Add(time.Hour))
	second := f.batch(t, product, "0.750", f.now.Add(24*time.Hour))
	f.batch(t, product, "2.000", f.now.Add(25*time.Hour))
	f.batch(t, product, "0.000", f.now.Add(time.Microsecond))
	f.batch(t, missing, "10.000", f.now.Add(-time.Microsecond))
	a := f.dish(t, "A available", true, testIngredient{product, "1.500"})
	b := f.dish(t, "B available", true, testIngredient{product, "1.500"})
	f.dish(t, "Inactive", false, testIngredient{product, "1.000"})
	f.dish(t, "Recipe missing", true)
	f.dish(t, "Blocked", true, testIngredient{product, "0.250"}, testIngredient{missing, "0.001"})
	f.dish(t, "Insufficient", true, testIngredient{product, "3.001"})
	latest := f.dish(t, "Latest recipe", true, testIngredient{product, "1.000"})
	f.recipe(t, latest, 2, testIngredient{missing, "1.000"})
	offers, hours := f.simulation(t)
	if hours != 24 || len(offers) != 2 {
		t.Fatalf("offers: %+v, threshold %d", offers, hours)
	}
	for i, id := range []uuid.UUID{a, b} {
		o := offers[i]
		if o.dish.ID != id || o.availablePortions != "2" || len(o.problems) != 2 ||
			o.problems[0].allocation.Batch.ID != first ||
			o.problems[1].allocation.Batch.ID != second ||
			o.problems[0].allocation.Quantity != "0.250" ||
			o.problems[1].allocation.Quantity != "0.750" {
			t.Fatalf("FEFO: %+v", o)
		}
		if o.problems[0].allocation.Batch.SupplyNumber != "S13-1" || o.problems[0].allocation.Batch.LineNumber != 2 {
			t.Fatal("missing supply facts")
		}
	}
	var amount string
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id=$1`, old).
		Scan(&amount); err != nil ||
		amount != "50.000" {
		t.Fatalf("expired stock: %s, %v", amount, err)
	}
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name,expiring_threshold_hours)VALUES('Test',1)`)
	offers, hours = f.simulation(t)
	if hours != 1 || len(offers) != 2 || len(offers[0].problems) != 1 {
		t.Fatalf("configured threshold: %+v, %d", offers, hours)
	}
	var writes int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT COUNT(*) FROM stock_movements)+(SELECT COUNT(*) FROM orders)+(SELECT COUNT(*) FROM audit_log)+(SELECT COUNT(*) FROM outbox_events)`).
		Scan(&writes); err != nil ||
		writes != 0 {
		t.Fatalf("read produced writes: %d, %v", writes, err)
	}
}
