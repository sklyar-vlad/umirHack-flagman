package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	producthandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/product"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	productrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/product"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	productservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/product"
	"go.uber.org/zap"
)

type productFixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	server   *httptest.Server
	actor    uuid.UUID
	category uuid.UUID
	ids      []uuid.UUID
	token    string
	created  time.Time
}

func newProductFixture(t *testing.T) *productFixture {
	t.Helper()
	dsn := os.Getenv("PRODUCT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PRODUCT_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := "products_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, file := range files {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(script), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
	}
	f := &productFixture{
		ctx: ctx, pool: pool, actor: uuid.New(), category: uuid.New(), token: uuid.NewString() + uuid.NewString(),
		created: time.Date(2026, 10, 10, 15, 0, 0, 123456000, time.FixedZone("MSK", 3*60*60)),
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,name,email,password_hash,role)
		VALUES ($1,'Оператор','products@example.test','fixture-hash','order_operator')`, f.actor.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (user_id,token_hash,csrf_token,expires_at)
		VALUES ($1,$2,'fixture-csrf',clock_timestamp()+interval '12 hours')`, f.actor.String(), authservice.SessionTokenHash(f.token)); err != nil {
		t.Fatal(err)
	}
	meat := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO product_categories (id,name) VALUES ($1,'Milk'),($2,'Meat')`,
		f.category.String(), meat.String()); err != nil {
		t.Fatal(err)
	}
	for i, p := range []struct {
		name, unit, display string
		category            uuid.UUID
		active              bool
		target              *string
		hours               *int
	}{
		{"Mozzarella", "g", "kg", f.category, true, strPtr("4000.125"), intPtr(72)},
		{"Mozzarella B", "g", "g", f.category, true, strPtr("99999999999.999"), intPtr(48)},
		{"Cream", "ml", "l", f.category, true, nil, nil},
		{"Chicken", "g", "kg", meat, true, strPtr("2.500"), intPtr(24)},
		{`Milk%_\`, "pcs", "pcs", f.category, true, strPtr("0.000"), nil},
		{"Inactive", "g", "g", f.category, false, nil, intPtr(24)},
	} {
		id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1))
		f.ids = append(f.ids, id)
		if _, err := pool.Exec(ctx, `INSERT INTO products
			(id,name,category_id,unit,display_unit,is_active,target_stock,standard_shelf_life_hours,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9)`, id.String(), p.name, p.category.String(), p.unit, p.display, p.active,
			p.target, p.hours, f.created); err != nil {
			t.Fatal(err)
		}
	}
	logger := zap.NewNop()
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	serv := productservice.NewService(productrepository.NewRepository(), repository.NewRepository(pool, logger))
	router := gin.New()
	router.Use(handler.RequestID(), handler.Recovery(logger), handler.BodyLimit(1<<20))
	handler.RegisterProductRoutes(
		router.Group("/api/v1"),
		producthandler.NewHandler(serv),
		authhandler.NewHandler(auth),
	)
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }

func (f *productFixture) request(t *testing.T, query url.Values, cookie bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		f.ctx,
		http.MethodGet,
		f.server.URL+"/api/v1/products?"+query.Encode(),
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cookie {
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
	}
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(response.Header.Get("X-Request-ID")); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 400 {
		var result dto.ErrorResponse
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if result.RequestID != response.Header.Get("X-Request-ID") {
			t.Fatal("error request_id mismatch")
		}
	}
	return response.StatusCode, body
}

func TestProductListHTTPIntegration(t *testing.T) {
	f := newProductFixture(t)
	for _, test := range []struct {
		name    string
		query   url.Values
		indices []int
		total   int64
	}{
		{"defaults", url.Values{}, []int{3, 2, 4, 0, 1}, 5},
		{"all", url.Values{"is_active": {"all"}}, []int{3, 2, 5, 4, 0, 1}, 6},
		{"inactive", url.Values{"is_active": {"false"}}, []int{5}, 1},
		{"search", url.Values{"search": {"mOzZ"}}, []int{0, 1}, 2},
		{"category", url.Values{"category_id": {f.category.String()}}, []int{2, 4, 0, 1}, 4},
		{"combined", url.Values{"category_id": {f.category.String()}, "search": {"mOzZ"}, "is_active": {"true"}}, []int{0, 1}, 2},
		{"percent literal", url.Values{"search": {"%"}}, []int{4}, 1},
		{"underscore literal", url.Values{"search": {"_"}}, []int{4}, 1},
		{"backslash literal", url.Values{"search": {`\`}}, []int{4}, 1},
		{"SQL text literal", url.Values{"search": {"%' OR true--"}}, []int{}, 0},
		{"unknown category", url.Values{"category_id": {uuid.NewString()}}, []int{}, 0},
		{"second page", url.Values{"page": {"2"}, "page_size": {"2"}}, []int{4, 0}, 5},
		{"past end", url.Values{"page": {"4"}, "page_size": {"2"}}, []int{}, 5},
		{"large safe offset", url.Values{"page": {"9223372036854775807"}, "page_size": {"1"}}, []int{}, 5},
		{"first similar name", url.Values{"page": {"4"}, "page_size": {"1"}, "sort": {"name"}}, []int{0}, 5},
		{"second similar name", url.Values{"page": {"5"}, "page_size": {"1"}}, []int{1}, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body := f.request(t, test.query, true)
			if status != http.StatusOK {
				t.Fatalf("%d: %s", status, body)
			}
			var result dto.ProductListResponse
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			if result.Items == nil || result.Meta == nil || result.Total != test.total {
				t.Fatal("incorrect envelope/total")
			}
			ids := make([]uuid.UUID, 0, len(result.Items))
			for i := range result.Items {
				item := &result.Items[i]
				ids = append(ids, item.ID)
				if item.Category.ID == uuid.Nil || item.Category.Name == "" || item.Revision != 1 ||
					item.CreatedAt != f.created.UTC().Format(time.RFC3339Nano) {
					t.Fatal("category or metadata missing")
				}
			}
			want := make([]uuid.UUID, 0, len(test.indices))
			for _, i := range test.indices {
				want = append(want, f.ids[i])
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("IDs %v; want %v", ids, want)
			}
		})
	}
	status, body := f.request(t, url.Values{"search": {"Mozzarella"}}, true)
	var result dto.ProductListResponse
	if status != 200 || json.Unmarshal(body, &result) != nil || len(result.Items) != 2 {
		t.Fatal("projection request failed")
	}
	first := &result.Items[0]
	if first.Name != "Mozzarella" || first.Unit != "g" || first.DisplayUnit != "kg" || first.Category.Name != "Milk" ||
		first.StandardShelfLifeHours == nil || *first.StandardShelfLifeHours != 72 || first.TargetStock == nil ||
		*first.TargetStock != "4000.125" || result.Items[1].TargetStock == nil || *result.Items[1].TargetStock != "99999999999.999" {
		t.Fatal("form projection lost fields or decimal precision")
	}
	_, body = f.request(t, url.Values{"search": {"Cream"}}, true)
	if !strings.Contains(string(body), `"standard_shelf_life_hours":null`) ||
		!strings.Contains(string(body), `"target_stock":null`) {
		t.Fatal("absent defaults must be explicit nulls")
	}
	_, body = f.request(t, url.Values{"search": {"%"}}, true)
	if !strings.Contains(string(body), `"target_stock":"0.000"`) {
		t.Fatal("zero target must not become null")
	}
}

func TestProductListAccessAndReadFailureIntegration(t *testing.T) {
	f := newProductFixture(t)
	status, body := f.request(t, url.Values{}, false)
	if status != http.StatusUnauthorized || !strings.Contains(string(body), "UNAUTHORIZED") {
		t.Fatal("missing session accepted")
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		if _, err := f.pool.Exec(f.ctx, `UPDATE users SET role=$2 WHERE id=$1`, f.actor.String(), role); err != nil {
			t.Fatal(err)
		}
		status, body = f.request(t, url.Values{}, true)
		if status != http.StatusOK {
			t.Fatalf("role %s: %d %s", role, status, body)
		}
		for _, secret := range []string{"fixture-hash", "fixture-csrf", f.token, "password", "token_hash"} {
			if strings.Contains(string(body), secret) {
				t.Fatal("product projection exposes secrets")
			}
		}
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE sessions SET revoked_at=clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	status, _ = f.request(t, url.Values{}, true)
	if status != http.StatusUnauthorized {
		t.Fatal("revoked session accepted")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE sessions SET revoked_at=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE product_categories RENAME COLUMN name TO hidden_name`); err != nil {
		t.Fatal(err)
	}
	status, body = f.request(t, url.Values{}, true)
	if status != http.StatusInternalServerError || !strings.Contains(string(body), "INTERNAL_ERROR") ||
		strings.Contains(string(body), "hidden_name") || strings.Contains(string(body), "Mozzarella") {
		t.Fatal("SQL failure did not produce a safe complete error")
	}
}

type countHookPool struct {
	*pgxpool.Pool
	afterCount func() error
}

//nolint:gocritic
func (p *countHookPool) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &countHookTx{Tx: tx, afterCount: p.afterCount}, nil
}

type countHookTx struct {
	pgx.Tx
	afterCount func() error
}

func (t *countHookTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return &countHookRow{Row: t.Tx.QueryRow(ctx, query, args...), afterCount: t.afterCount}
}

type countHookRow struct {
	pgx.Row
	afterCount func() error
}

func (r *countHookRow) Scan(args ...any) error {
	if err := r.Row.Scan(args...); err != nil {
		return err
	}
	return r.afterCount()
}

func TestProductListCountAndPageUseSameSnapshotIntegration(t *testing.T) {
	f := newProductFixture(t)
	pool := &countHookPool{Pool: f.pool, afterCount: func() error {
		_, err := f.pool.Exec(f.ctx, `UPDATE products SET is_active=false WHERE id=$1`, f.ids[2].String())
		return err
	}}
	serv := productservice.NewService(productrepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	active := true
	items, total, err := serv.ListProducts(f.ctx, model.ProductFilter{IsActive: &active, Page: 1, PageSize: 50})
	if err != nil || total != 5 || len(items) != 5 {
		t.Fatalf("inconsistent snapshot: total %d, items %d, err %v", total, len(items), err)
	}
	found := false
	for i := range items {
		if items[i].ID == f.ids[2] {
			found = true
		}
	}
	if !found {
		t.Fatal("page lost a product present in the count snapshot")
	}
	var current int
	if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM products WHERE is_active`).
		Scan(&current); err != nil ||
		current != 4 {
		t.Fatalf("concurrent change did not commit: %d, %v", current, err)
	}
}
