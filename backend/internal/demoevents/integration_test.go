package demoevents

import (
	"bytes"
	"context"
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
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	menuhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/menu"
	orderhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/order"
	supplyhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	supplyrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	menuservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func demoIntegrationFixture(t *testing.T) (context.Context, *pgxpool.Pool, *Client) {
	t.Helper()
	dsn := os.Getenv("FLAGMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FLAGMAN_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid dedicated test database configuration")
	}
	if (cfg.ConnConfig.Host != "localhost" && cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1") || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("requires dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	name := "demo_events_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	files, err := filepath.Glob("../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("missing migrations")
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	user := uuid.New()
	hash, err := authservice.Hash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Demo Manager',$2,$3,'manager')`, user, demoEmail, hash)
	exec(`INSERT INTO restaurant_settings(restaurant_name) VALUES('Demo test')`)
	category := uuid.New()
	exec(`INSERT INTO product_categories(id,name) VALUES($1,'Test')`, category)
	recipe := uuid.New()
	exec(`INSERT INTO dishes(id,name) VALUES($1,'Demo dish')`, testDish.ID)
	exec(`INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, recipe, testDish.ID, user)
	for _, item := range []struct{ unit, display, quantity string }{{"g", "kg", "0.125"}, {"ml", "l", "0.375"}, {"pcs", "pcs", "1.000"}} {
		product := uuid.New()
		exec(`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,$3,$4,$5)`, product, category, "Test "+item.unit, item.unit, item.display)
		exec(`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,$3)`, recipe, product, item.quantity)
	}
	shared := repository.NewRepository(pool, zap.NewNop())
	idem := idempotency.NewService(shared)
	auth, err := authservice.NewService(authrepo.NewRepository(pool), "http://frontend.test")
	if err != nil {
		t.Fatal(err)
	}
	access := authhandler.NewHandler(auth)
	menu := menuhandler.NewHandler(menuservice.NewService(menurepo.NewRepository(), shared))
	check := orderhandler.NewHandler(orderservice.NewService(menurepo.NewRepository(), shared))
	create := orderhandler.NewCreateHandler(orderservice.NewCreateService(orderrepo.NewRepository(), shared, idem))
	supplies := supplyservice.NewService(supplyrepo.NewRepository(), shared, idem)
	supply := supplyhandler.NewHandler(service.NewService(shared, zap.NewNop(), supplies))
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(access.RequireRouteAccess)
	api.POST("/auth/login", access.Login)
	api.POST("/auth/logout", access.Logout)
	api.GET("/dishes", menu.ListDishes)
	api.GET("/dishes/:id", menu.GetDish)
	api.POST("/orders/check", check.Check)
	api.POST("/orders", create.Create)
	api.POST("/supplies", supply.CreateSupply)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client, err := NewClient("demo", server.URL, "http://frontend.test")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, client
}

func TestDemoScenariosThroughAuthenticatedAPIsOnPostgreSQL(t *testing.T) {
	ctx, pool, client := demoIntegrationFixture(t)
	if err := client.Login(ctx, "test-password"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, scenario := range []string{"normal", "normal", "replenishment", "depletion", "depletion", "expiring"} {
		if err := client.Step(ctx, testDish.ID.String(), scenario, 2*time.Second, &out); err != nil {
			t.Fatalf("%s: %v", scenario, err)
		}
	}
	var orders, receipts, attempts, keys, audits, outbox int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM supplies),(SELECT count(*) FROM order_attempts),(SELECT count(*) FROM idempotency_keys),(SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events)`).Scan(&orders, &receipts, &attempts, &keys, &audits, &outbox); err != nil {
		t.Fatal(err)
	}
	if orders != 2 || receipts != 3 || attempts != 1 || keys != 6 || audits != 5 || outbox != 5 {
		t.Fatalf("incorrect API facts: orders=%d receipts=%d attempts=%d keys=%d audits=%d outbox=%d", orders, receipts, attempts, keys, audits, outbox)
	}
	var stock string
	if err := pool.QueryRow(ctx, `SELECT sum(remaining_quantity)::text FROM batches WHERE product_id=(SELECT id FROM products WHERE unit='pcs')`).Scan(&stock); err != nil || stock != "1.000" {
		t.Fatalf("exact stock: %s %v", stock, err)
	}
	timer := time.NewTimer(2200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	dish, err := client.dish(ctx, testDish.ID.String())
	if err != nil || dish.IsAvailable || dish.AvailablePortions != "0" {
		t.Fatalf("batch did not expire in real menu: %+v %v", dish, err)
	}
	if err := client.Step(ctx, testDish.ID.String(), "depletion", 30*time.Second, &out); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_attempts`).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatal("expiry refusal not saved through API")
	}
	if err := client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE revoked_at IS NULL`).Scan(&active); err != nil || active != 0 {
		t.Fatal("demo session not revoked")
	}
	if strings.Count(out.String(), "order created id=") != 2 || strings.Count(out.String(), "supply created id=") != 3 || strings.Count(out.String(), "refused reason=INSUFFICIENT_STOCK") != 2 || strings.Count(out.String(), "batch id=") != 9 || strings.Contains(out.String(), "test-password") {
		t.Fatalf("incorrect result output: %s", out.String())
	}
}

func TestBoundedDemoRunnerUsesOnlyBusinessAPIs(t *testing.T) {
	ctx, pool, client := demoIntegrationFixture(t)
	var out bytes.Buffer
	if err := client.Run(ctx, "test-password", Options{Scenario: "normal", Count: 4, Interval: time.Millisecond, Expiry: 30 * time.Second}, &out); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err := pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM supplies)=2 AND (SELECT count(*) FROM orders)=2
 AND (SELECT count(*) FROM idempotency_keys)=4
 AND (SELECT count(*) FROM audit_log)=4 AND (SELECT count(*) FROM outbox_events)=4
 AND (SELECT count(*) FROM stock_movements WHERE type='receipt')=6
 AND (SELECT count(*) FROM stock_movements WHERE type='order_consumption')=6
 AND NOT EXISTS(SELECT 1 FROM batches WHERE remaining_quantity<>0)
 AND NOT EXISTS(SELECT 1 FROM sessions WHERE revoked_at IS NULL)`).Scan(&valid); err != nil || !valid {
		t.Fatalf("bounded API facts: %v %v", valid, err)
	}
	if strings.Count(out.String(), " created id=") != 4 {
		t.Fatalf("unexpected event count %s", out.String())
	}
}
