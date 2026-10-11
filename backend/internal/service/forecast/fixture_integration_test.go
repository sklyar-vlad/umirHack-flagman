package forecast

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type fixture struct {
	ctx                                                        context.Context
	pool                                                       *pgxpool.Pool
	now                                                        time.Time
	user, category, product, supply, batch, dish, recipe, item uuid.UUID
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
	if (cfg.ConnConfig.Host != "localhost" && cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1") || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("forecast tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	name := "forecast_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
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
	f := &fixture{ctx: ctx, pool: pool, now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC), user: uuid.New(), category: uuid.New(), product: uuid.New(), supply: uuid.New(), batch: uuid.New(), dish: uuid.New(), recipe: uuid.New(), item: uuid.New()}
	f.exec(t, `INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Manager','manager@example.test','unused','manager')`, f.user)
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Ingredients')`, f.category)
	f.exec(t, `INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Ingredient','g','g')`, f.product, f.category)
	f.exec(t, `INSERT INTO supplies(id,number,created_by,received_at) VALUES($1,1,$2,$3)`, f.supply, f.user, f.now.Add(-time.Hour))
	f.exec(t, `INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,100,100,'explicit',$4)`, f.batch, f.product, f.supply, f.now.Add(1000*time.Hour))
	f.exec(t, `INSERT INTO dishes(id,name) VALUES($1,'Dish')`, f.dish)
	f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, f.recipe, f.dish, f.user)
	f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,10)`, f.recipe, f.product)
	order := uuid.New()
	f.exec(t, `INSERT INTO orders(id,created_by) VALUES($1,$2)`, order, f.user)
	f.exec(t, `INSERT INTO order_items(id,order_id,dish_id,recipe_version_id,dish_name_snapshot,portions) VALUES($1,$2,$3,$4,'Dish',1)`, f.item, order, f.dish, f.recipe)
	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) actor() context.Context {
	return requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: "manager"})
}
func (f *fixture) consumption(t *testing.T, amount, unit string, at time.Time) {
	f.exec(t, `INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,order_item_id,occurred_at) VALUES($1,'order_consumption',-$2::numeric,0,'Historical ingredient',$3,$4,$5,$6)`, f.batch, amount, unit, f.user, f.item, at)
}
