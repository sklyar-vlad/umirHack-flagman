package supply_test

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
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	supplyhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service"
	"go.uber.org/zap"
)

type listFixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	server *httptest.Server
	actor  uuid.UUID
	ids    []uuid.UUID
	base   time.Time
}

func newListFixture(t *testing.T) *listFixture {
	t.Helper()
	dsn := os.Getenv("SUPPLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPLY_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := "supplies_list_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	t.Cleanup(func() { pool.Close() })
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, file := range files {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(script), "-- +goose Down", 2)[0]); err != nil {
			t.Fatalf("migrate %s: %v", file, err)
		}
	}
	f := &listFixture{
		ctx: ctx, pool: pool, actor: uuid.New(),
		base: time.Date(2026, 10, 10, 12, 0, 0, 123456000, time.UTC),
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,name,email,password_hash,role)
		VALUES ($1,'Кладовщик','list@example.test','fixture-hash','storekeeper')`, f.actor.String()); err != nil {
		t.Fatal(err)
	}
	var category, product uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO product_categories (name) VALUES ('Тест') RETURNING id`).
		Scan(&category); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (category_id,name,unit,display_unit)
		VALUES ($1,'Моцарелла','g','kg') RETURNING id`, category.String()).Scan(&product); err != nil {
		t.Fatal(err)
	}
	numbers := []string{"IN-104", "in-105", `IN%_\106`, "IN-107", "OTHER-104", "IN'108"}
	hours := []int{0, 1, 1, 2, 3, 4}
	for i, number := range numbers {
		id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1))
		f.ids = append(f.ids, id)
		created := f.base.Add(time.Duration(hours[i]) * time.Hour)
		// Different receipt dates detect accidental filtering/sorting by received_at.
		received := f.base.Add(-time.Duration(i) * time.Hour)
		if _, err := pool.Exec(ctx, `INSERT INTO supplies (id,number,created_at,received_at,created_by)
			VALUES ($1,$2,$3,$4,$5)`, id.String(), number, created, received, f.actor.String()); err != nil {
			t.Fatal(err)
		}
		for line := 1; line <= 1+i%2; line++ {
			if _, err := pool.Exec(ctx, `INSERT INTO batches
				(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)
				VALUES ($1,$2,$3,1,$4,'explicit',$5)`, id.String(), product.String(), line, line%2,
				f.base.Add(48*time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
	}
	logger := zap.NewNop()
	serv := service.NewService(repository.NewRepository(pool, logger), logger)
	router := gin.New()
	router.Use(handler.RequestID(), handler.Recovery(logger), handler.RequestLogger(logger),
		handler.RateLimit(10, 20), handler.BodyLimit(1<<20))
	router.NoRoute(handler.NotFound)
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api, handler.NewHandler(serv, logger))
	handler.RegisterSupplyRoutes(api, supplyhandler.NewHandler(serv))
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func (f *listFixture) list(t *testing.T, query url.Values) supplydto.SupplyListResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(
		f.ctx,
		http.MethodGet,
		f.server.URL+"/api/v1/supplies?"+query.Encode(),
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
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
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d; body %s", response.StatusCode, body)
	}
	if _, err := uuid.Parse(response.Header.Get("X-Request-ID")); err != nil {
		t.Fatal("request ID missing")
	}
	if strings.Contains(string(body), "fixture-hash") || strings.Contains(string(body), "password_hash") {
		t.Fatal("list exposes secrets")
	}
	var result supplydto.SupplyListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || result.Meta == nil {
		t.Fatal("items and meta must not be null")
	}
	return result
}

func TestSupplyListHTTPIntegration(t *testing.T) {
	f := newListFixture(t)
	date := func(hours int) string { return f.base.Add(time.Duration(hours) * time.Hour).Format(time.RFC3339Nano) }
	for _, test := range []struct {
		name    string
		query   url.Values
		indices []int
		total   int64
	}{
		{"defaults", url.Values{}, []int{5, 4, 3, 2, 1, 0}, 6},
		{"second page", url.Values{"page": {"2"}, "page_size": {"2"}}, []int{3, 2}, 6},
		{"page past end", url.Values{"page": {"4"}, "page_size": {"2"}}, []int{}, 6},
		{"huge safe offset", url.Values{"page": {"9223372036854775807"}, "page_size": {"1"}}, []int{}, 6},
		{"case insensitive search", url.Values{"search": {"iN"}}, []int{5, 3, 2, 1, 0}, 5},
		{"percent literal", url.Values{"search": {"%"}}, []int{2}, 1},
		{"underscore literal", url.Values{"search": {"_"}}, []int{2}, 1},
		{"backslash literal", url.Values{"search": {`\`}}, []int{2}, 1},
		{"quote literal", url.Values{"search": {"'"}}, []int{5}, 1},
		{"SQL text literal", url.Values{"search": {"%' OR true--"}}, []int{}, 0},
		{"combined half open interval", url.Values{"search": {"IN"}, "from": {date(1)}, "to": {date(2)}}, []int{2, 1}, 2},
		{"from only", url.Values{"from": {date(3)}}, []int{5, 4}, 2},
		{"to only", url.Values{"to": {date(1)}}, []int{0}, 1},
		{"nanosecond from boundary", url.Values{"from": {f.base.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, []int{5, 4, 3, 2, 1}, 5},
		{"nanosecond to boundary", url.Values{"to": {f.base.Add(time.Nanosecond).Format(time.RFC3339Nano)}}, []int{0}, 1},
		{"equal boundaries", url.Values{"from": {date(1)}, "to": {date(1)}}, []int{}, 0},
		{"empty search", url.Values{"search": {""}}, []int{5, 4, 3, 2, 1, 0}, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := f.list(t, test.query)
			want := make([]uuid.UUID, 0, len(test.indices))
			for _, i := range test.indices {
				want = append(want, f.ids[i])
			}
			got := make([]uuid.UUID, 0, len(result.Items))
			for _, item := range result.Items {
				got = append(got, item.ID)
				if item.CreatedBy.ID != f.actor || item.CreatedBy.Name != "Кладовщик" || item.Status != "received" {
					t.Fatal("summary actor/status is missing")
				}
			}
			if !reflect.DeepEqual(want, got) || result.Total != test.total {
				t.Fatalf("IDs %v, total %d; want %v, %d", got, result.Total, want, test.total)
			}
		})
	}
	// Equal creation times are split across pages in a repeatable ID order.
	for range 2 {
		page := f.list(t, url.Values{"page": {"2"}, "page_size": {"3"}})
		if len(page.Items) != 3 || page.Items[0].ID != f.ids[2] || page.Items[1].ID != f.ids[1] {
			t.Fatal("tie ordering changed across pages")
		}
	}
}

type countHookPool struct {
	*pgxpool.Pool
	afterCount func() error
}

//nolint:gocritic // The signature must match pgxpool.Pool.BeginTx.
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
	row := t.Tx.QueryRow(ctx, query, args...)
	if strings.Contains(query, "SELECT COUNT(*)") {
		return &countHookRow{Row: row, afterCount: t.afterCount}
	}
	return row
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

func TestSupplyListCountAndPageUseSameSnapshotIntegration(t *testing.T) {
	f := newListFixture(t)
	pool := &countHookPool{Pool: f.pool, afterCount: func() error {
		_, err := f.pool.Exec(f.ctx, `INSERT INTO supplies (number,received_at,created_by)
			VALUES ('committed-between-reads', $1, $2)`, f.base, f.actor.String())
		return err
	}}
	repo := repository.NewRepository(pool, zap.NewNop())
	items, total, err := repo.ListSupplies(f.ctx, model.SupplyFilter{Page: 1, PageSize: 50})
	if err != nil || total != 6 || len(items) != 6 {
		t.Fatalf("inconsistent count/page: %d/%d, %v", total, len(items), err)
	}
	for _, item := range items {
		if item.Number == "committed-between-reads" {
			t.Fatal("page sees a receipt outside the count snapshot")
		}
	}
	var current int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM supplies").Scan(&current); err != nil || current != 7 {
		t.Fatalf("concurrent insert was not committed: %d, %v", current, err)
	}
}
