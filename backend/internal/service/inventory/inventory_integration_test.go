package inventory

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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	inventoryhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/inventory"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	inventoryrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/inventory"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type inventoryFixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	service  *Service
	server   *httptest.Server
	actor    uuid.UUID
	category uuid.UUID
	token    string
	now      time.Time
	clockMu  sync.Mutex
	sequence int
}

func newInventoryFixture(t *testing.T) *inventoryFixture {
	t.Helper()
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := "inventory_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &inventoryFixture{
		ctx: ctx, pool: pool, actor: uuid.New(), category: uuid.New(), token: uuid.NewString() + uuid.NewString(),
		now: time.Now().UTC().Truncate(time.Microsecond),
	}
	f.exec(t, `INSERT INTO users (id,name,email,password_hash,role)
		VALUES ($1,'Оператор','inventory@example.test','fixture-hash','order_operator')`, f.actor.String())
	f.exec(t, `INSERT INTO sessions (user_id,token_hash,csrf_token,expires_at)
		VALUES ($1,$2,'fixture-csrf',clock_timestamp()+interval '12 hours')`, f.actor.String(), authservice.SessionTokenHash(f.token))
	f.exec(t, `INSERT INTO product_categories (id,name) VALUES ($1,'Milk')`, f.category.String())
	logger := zap.NewNop()
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	f.service = NewService(inventoryrepository.NewRepository(), repository.NewRepository(pool, logger))
	f.service.now = func() time.Time {
		f.clockMu.Lock()
		defer f.clockMu.Unlock()
		return f.now
	}
	router := gin.New()
	router.Use(handler.RequestID(), handler.Recovery(logger))
	handler.RegisterInventoryRoutes(
		router.Group("/api/v1"),
		inventoryhandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func (f *inventoryFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *inventoryFixture) product(t *testing.T, name, unit string, target *string, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO products (id,name,category_id,unit,display_unit,target_stock,is_active)
		VALUES ($1,$2,$3,$4::text::product_unit,$4::text::display_unit,$5::numeric,$6)`,
		id.String(), name, f.category.String(), unit, target, active)
	return id
}

// Every batch has its own supply, including repeated lines for the same product.
// Expired stock is a fixture; receipt validation is deliberately not bypassed in production.
func (f *inventoryFixture) batch(t *testing.T, product uuid.UUID, quantity string, expiry time.Time) uuid.UUID {
	t.Helper()
	f.sequence++
	id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", f.sequence))
	supply := uuid.New()
	err := repository.NewRepository(f.pool, zap.NewNop()).WithinTx(f.ctx, nil, func(q repository.DBTX) error {
		if _, err := q.Exec(f.ctx, `INSERT INTO supplies (id,number,received_at,created_by)
			VALUES ($1,$2,$3,$4)`, supply.String(), fmt.Sprintf("INV-%d", f.sequence), f.now.Add(-72*time.Hour), f.actor.String()); err != nil {
			return err
		}
		if _, err := q.Exec(f.ctx, `INSERT INTO batches
			(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)
			VALUES ($1,$2,$3,1,$4::numeric,$4::numeric,'explicit',$5)`, id.String(), supply.String(), product.String(), quantity, expiry); err != nil {
			return err
		}
		_, err := q.Exec(f.ctx, `INSERT INTO stock_movements
			(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at)
			SELECT b.id,'receipt',b.initial_quantity,b.initial_quantity,p.name,p.unit,$2,$3
			FROM batches b JOIN products p ON p.id=b.product_id WHERE b.id=$1`, id.String(), f.actor.String(), f.now.Add(-72*time.Hour))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *inventoryFixture) consume(t *testing.T, batch uuid.UUID, remaining string) {
	t.Helper()
	err := repository.NewRepository(f.pool, zap.NewNop()).WithinTx(f.ctx, nil, func(q repository.DBTX) error {
		if _, err := q.Exec(f.ctx, `INSERT INTO stock_movements
			(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note)
			SELECT b.id,'manual_writeoff',$2::numeric-b.remaining_quantity,$2::numeric,p.name,p.unit,$3,$4,'Test consumption'
			FROM batches b JOIN products p ON p.id=b.product_id WHERE b.id=$1`, batch.String(), remaining, f.actor.String(), f.now); err != nil {
			return err
		}
		_, err := q.Exec(
			f.ctx,
			`UPDATE batches SET remaining_quantity=$2::numeric WHERE id=$1`,
			batch.String(),
			remaining,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *inventoryFixture) request(t *testing.T, query url.Values, authenticated bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		f.ctx,
		http.MethodGet,
		f.server.URL+"/api/v1/inventory?"+query.Encode(),
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated {
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
		if err := json.Unmarshal(body, &result); err != nil || result.RequestID != response.Header.Get("X-Request-ID") {
			t.Fatalf("error envelope: %s (%v)", body, err)
		}
	}
	return response.StatusCode, body
}

func (f *inventoryFixture) list(t *testing.T, query url.Values) *dto.InventoryListResponse {
	t.Helper()
	status, body := f.request(t, query, true)
	var result dto.InventoryListResponse
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("inventory: %d %s", status, body)
	}
	if result.Items == nil || result.AttentionBatches == nil ||
		result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("missing arrays or inconsistent clock: %s", body)
	}
	return &result
}

func stringPtr(value string) *string { return &value }

func TestInventoryAggregationAndExpiryBoundariesIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	product := f.product(t, "Mozzarella", "g", stringPtr("20.000"), true)
	f.batch(t, product, "1.250", f.now.Add(24*time.Hour+time.Microsecond))
	nearest := f.batch(t, product, "2.125", f.now.Add(time.Microsecond))
	f.batch(t, product, "3.000", f.now.Add(24*time.Hour))
	f.batch(t, product, "4.500", f.now)
	f.batch(t, product, "5.125", f.now.Add(-time.Microsecond))
	depleted := f.batch(t, product, "100.000", f.now.Add(12*time.Hour))
	f.consume(t, depleted, "0.000")
	depletedExpired := f.batch(t, product, "100.000", f.now.Add(-2*time.Microsecond))
	f.consume(t, depletedExpired, "0.000")
	result := f.list(t, nil)
	item := &result.Items[0]
	if result.Total != 1 || item.PhysicalQuantity != "16.000" || item.AvailableQuantity != "6.375" ||
		item.ExpiringQuantity != "5.125" || item.ExpiredQuantity != "9.625" || item.BatchesCount != 5 ||
		!item.HasExpiring || !item.HasExpired || item.IsOutOfStock || item.Status != "expired" ||
		item.NearestExpiresAt == nil || *item.NearestExpiresAt != f.now.Add(time.Microsecond).Format(time.RFC3339Nano) ||
		item.RemainingSeconds == nil || *item.RemainingSeconds != 0 || item.DeficitQuantity == nil || *item.DeficitQuantity != "13.625" {
		t.Fatalf("incorrect aggregate: %+v", item)
	}
	wantCounts := dto.InventoryCounts{All: 1, Expiring: 1, Expired: 1, Deficit: 1}
	if result.Counts != wantCounts || result.AttentionTotal != 4 ||
		len(result.AttentionBatches) != 4 || result.AttentionBatches[0].RemainingSeconds != -1 ||
		result.AttentionBatches[1].RemainingSeconds != 0 || result.AttentionBatches[1].Status != "expired" ||
		result.AttentionBatches[2].BatchID != nearest ||
		result.AttentionBatches[2].Status != "expiring" {
		t.Fatalf("incorrect overlapping counts/attention: %+v", result)
	}
	// Time alone expires another batch; physical stock and batch count stay unchanged.
	f.clockMu.Lock()
	f.now = f.now.Add(time.Microsecond)
	f.clockMu.Unlock()
	item = &f.list(t, nil).Items[0]
	if item.AvailableQuantity != "4.250" || item.ExpiringQuantity != "4.250" || item.ExpiredQuantity != "11.750" ||
		item.PhysicalQuantity != "16.000" || item.BatchesCount != 5 {
		t.Fatalf("expiry was not recalculated: %+v", item)
	}
	// A product's standard shelf life must never replace the batch's actual expiry.
	f.exec(t, `UPDATE products SET standard_shelf_life_hours=1 WHERE id=$1`, product.String())
	if got := f.list(t, nil).Items[0].AvailableQuantity; got != "4.250" {
		t.Fatalf("product settings changed actual batch expiry: %s", got)
	}
}

func TestInventoryEmptyExpiredInactiveAndLargeTotalsIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	f.product(t, "Chicken", "g", stringPtr("5.000"), true)
	cream := f.product(t, "Cream", "ml", nil, true)
	f.batch(t, cream, "8.000", f.now.Add(-time.Hour))
	inactive := f.product(t, "Inactive", "pcs", stringPtr("0.000"), false)
	f.batch(t, inactive, "2.000", f.now.Add(72*time.Hour))
	large := f.product(t, "Large", "g", stringPtr("1.000"), true)
	f.batch(t, large, "99999999999.999", f.now.Add(72*time.Hour))
	f.batch(t, large, "99999999999.999", f.now.Add(96*time.Hour))
	result := f.list(t, nil)
	if result.Counts != (dto.InventoryCounts{All: 4, Expired: 1, OutOfStock: 2, Deficit: 1}) || len(result.Items) != 4 {
		t.Fatalf("missing empty or inactive products: %+v", result)
	}
	chicken, expired, disabled, huge := &result.Items[0], &result.Items[1], &result.Items[2], &result.Items[3]
	if chicken.PhysicalQuantity != "0.000" ||
		chicken.AvailableQuantity != "0.000" ||
		chicken.ExpiringQuantity != "0.000" ||
		chicken.ExpiredQuantity != "0.000" ||
		chicken.BatchesCount != 0 ||
		chicken.NearestExpiresAt != nil ||
		chicken.RemainingSeconds != nil ||
		chicken.Status != "out_of_stock" ||
		*chicken.DeficitQuantity != "5.000" {
		t.Fatalf("incorrect empty product: %+v", chicken)
	}
	if expired.Status != "expired" || !expired.IsOutOfStock || expired.PhysicalQuantity != "8.000" ||
		expired.BatchesCount != 1 || expired.NearestExpiresAt != nil || expired.TargetStock != nil || expired.DeficitQuantity != nil {
		t.Fatalf("incorrect expired-only product: %+v", expired)
	}
	if disabled.Product.IsActive || disabled.Product.Unit != "pcs" || disabled.Status != "normal" ||
		*disabled.TargetStock != "0.000" || *disabled.DeficitQuantity != "0.000" || huge.PhysicalQuantity != "199999999999.998" ||
		huge.AvailableQuantity != "199999999999.998" || *huge.DeficitQuantity != "0.000" || huge.BatchesCount != 2 {
		t.Fatalf("inactive/precision/deficit error: %+v %+v", disabled, huge)
	}
	_, body := f.request(t, url.Values{"search": {"Cream"}}, true)
	for _, field := range []string{"target_stock", "deficit_quantity", "nearest_expires_at", "remaining_seconds"} {
		if !strings.Contains(string(body), `"`+field+`":null`) {
			t.Fatalf("missing explicit null %s: %s", field, body)
		}
	}
}

func TestInventoryPartialAndFullConsumptionIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	product := f.product(t, "Milk", "ml", nil, true)
	first := f.batch(t, product, "10.125", f.now.Add(time.Hour))
	second := f.batch(t, product, "20.875", f.now.Add(72*time.Hour))
	f.consume(t, first, "2.125")
	item := &f.list(t, nil).Items[0]
	if item.PhysicalQuantity != "23.000" || item.AvailableQuantity != "23.000" || item.ExpiringQuantity != "2.125" ||
		item.BatchesCount != 2 || item.Status != "expiring" {
		t.Fatalf("partial consumption: %+v", item)
	}
	f.consume(t, first, "0.000")
	result := f.list(t, nil)
	item = &result.Items[0]
	if item.PhysicalQuantity != "20.875" || item.BatchesCount != 1 || item.Status != "normal" ||
		result.AttentionTotal != 0 ||
		item.NearestExpiresAt == nil ||
		*item.NearestExpiresAt != f.now.Add(72*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("full first batch consumption: %+v", item)
	}
	f.consume(t, second, "0.000")
	item = &f.list(t, nil).Items[0]
	if item.PhysicalQuantity != "0.000" || item.BatchesCount != 0 || item.NearestExpiresAt != nil ||
		item.RemainingSeconds != nil || !item.IsOutOfStock || item.HasExpiring || item.HasExpired {
		t.Fatalf("fully consumed product: %+v", item)
	}
	var inconsistent int
	if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM batches b WHERE b.remaining_quantity !=
		(SELECT SUM(delta_quantity) FROM stock_movements WHERE batch_id=b.id)`).Scan(&inconsistent); err != nil || inconsistent != 0 {
		t.Fatalf("fixture journal mismatch: %d, %v", inconsistent, err)
	}
}

func TestInventoryThresholdFiltersAndPaginationIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	f.exec(t, `INSERT INTO restaurant_settings (restaurant_name,expiring_threshold_hours) VALUES ('Test',48)`)
	mixed := f.product(t, "Milk", "ml", stringPtr("10.000"), true)
	f.batch(t, mixed, "3.000", f.now.Add(48*time.Hour))
	f.batch(t, mixed, "1.000", f.now.Add(-time.Hour))
	literal := f.product(t, `Milk%_\`, "g", nil, true)
	f.batch(t, literal, "2.000", f.now.Add(48*time.Hour+time.Microsecond))
	f.product(t, "Chicken", "g", stringPtr("1.000"), true)
	otherCategory := uuid.New()
	f.exec(t, `INSERT INTO product_categories (id,name) VALUES ($1,'Meat')`, otherCategory.String())
	f.exec(t, `UPDATE products SET category_id=$1 WHERE name='Chicken'`, otherCategory.String())
	for _, test := range []struct {
		name  string
		query url.Values
		total int64
		all   int64
		items int
	}{
		{"all", nil, 3, 3, 3},
		{"expiring", url.Values{"filter": {"expiring"}}, 1, 3, 1},
		{"expired", url.Values{"filter": {"expired"}}, 1, 3, 1},
		{"out_of_stock", url.Values{"filter": {"out_of_stock"}}, 1, 3, 1},
		{"deficit", url.Values{"filter": {"deficit"}}, 2, 3, 2},
		{"page", url.Values{"page": {"2"}, "page_size": {"2"}}, 3, 3, 1},
		{"past end", url.Values{"page": {"10"}, "page_size": {"2"}}, 3, 3, 0},
		{"category", url.Values{"category_id": {f.category.String()}}, 2, 2, 2},
		{"search", url.Values{"search": {"mIlK"}}, 2, 2, 2},
		{"literal percent", url.Values{"search": {"%"}}, 1, 1, 1},
		{"literal underscore", url.Values{"search": {"_"}}, 1, 1, 1},
		{"literal backslash", url.Values{"search": {`\`}}, 1, 1, 1},
		{"unknown category", url.Values{"category_id": {uuid.NewString()}}, 0, 0, 0},
		{"SQL literal", url.Values{"search": {"%' OR true--"}}, 0, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := f.list(t, test.query)
			if result.Total != test.total || result.Counts.All != test.all || len(result.Items) != test.items {
				t.Fatalf("filtered envelope: %+v", result)
			}
			if test.all >= 2 &&
				(result.AttentionTotal != 2 || result.Counts.Expiring != 1 || result.Counts.Expired != 1) {
				t.Fatal("filter/page incorrectly affected counts or attention")
			}
			if test.all <= 1 && result.AttentionTotal != 0 {
				t.Fatal("search/category did not filter attention")
			}
		})
	}
	result := f.list(t, url.Values{"sort": {"nearest_expires_at"}})
	if result.Items[0].Product.ID != mixed || result.Items[1].Product.ID != literal ||
		result.Items[2].NearestExpiresAt != nil {
		t.Fatal("nearest expiry sort or NULLS LAST is incorrect")
	}
	// At an identical nearest expiry the ID determines page order.
	f.exec(t, `UPDATE batches SET expires_at=$1 WHERE product_id=$2`, f.now.Add(48*time.Hour), literal.String())
	result = f.list(t, url.Values{"sort": {"nearest_expires_at"}, "page_size": {"1"}})
	next := f.list(t, url.Values{"sort": {"nearest_expires_at"}, "page_size": {"1"}, "page": {"2"}})
	if strings.Compare(result.Items[0].Product.ID.String(), next.Items[0].Product.ID.String()) >= 0 {
		t.Fatal("expiry ties do not use stable ID order")
	}
}

func TestInventoryAttentionLimitAndReadAccessIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	product := f.product(t, "Milk", "g", nil, true)
	want := make([]uuid.UUID, 0, 20)
	for i := range 25 {
		id := f.batch(t, product, "1.000", f.now.Add(-time.Hour))
		if i < 20 {
			want = append(want, id)
		}
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor.String(), role)
		result := f.list(t, url.Values{"filter": {"expiring"}})
		got := make([]uuid.UUID, 0, len(result.AttentionBatches))
		for i := range result.AttentionBatches {
			batch := &result.AttentionBatches[i]
			got = append(got, batch.BatchID)
			if batch.Status != "expired" || batch.RemainingSeconds != -3600 || batch.ProductID != product ||
				batch.LineNumber != 1 {
				t.Fatalf("attention batch details: %+v", batch)
			}
		}
		if result.Total != 0 || result.AttentionTotal != 25 || !reflect.DeepEqual(got, want) {
			t.Fatalf("attention limit/count/order: %+v", result)
		}
		_, body := f.request(t, nil, true)
		for _, secret := range []string{"fixture-hash", "fixture-csrf", f.token, "password", "token_hash"} {
			if strings.Contains(string(body), secret) {
				t.Fatal("inventory exposes secrets")
			}
		}
	}
	if status, _ := f.request(t, nil, false); status != http.StatusUnauthorized {
		t.Fatal("missing session accepted")
	}
	f.exec(t, `UPDATE sessions SET revoked_at=clock_timestamp()`)
	if status, _ := f.request(t, nil, true); status != http.StatusUnauthorized {
		t.Fatal("revoked session accepted")
	}
	f.exec(t, `UPDATE sessions SET revoked_at=NULL`)
	f.exec(t, `ALTER TABLE supplies RENAME COLUMN number TO hidden_number`)
	status, body := f.request(t, nil, true)
	if status != http.StatusInternalServerError || !strings.Contains(string(body), "INTERNAL_ERROR") ||
		strings.Contains(string(body), "Milk") || strings.Contains(string(body), "hidden_number") {
		t.Fatalf("late SQL failure leaked a partial response: %d %s", status, body)
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
	if strings.Contains(query, "FROM aggregated") {
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

func TestInventoryCountsPageAndAttentionShareSnapshotIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	product := f.product(t, "Milk", "g", nil, true)
	batch := f.batch(t, product, "1.000", f.now.Add(time.Hour))
	pool := &countHookPool{Pool: f.pool, afterCount: func() error {
		f.consume(t, batch, "0.000")
		return nil
	}}
	s := NewService(inventoryrepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	s.now = func() time.Time { return f.now }
	result, err := s.ListInventory(
		f.ctx,
		model.InventoryFilter{Page: 1, PageSize: 50, Filter: "expiring", Sort: "name"},
	)
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].AvailableQuantity != "1.000" ||
		result.Counts.Expiring != 1 || result.Counts.OutOfStock != 0 || result.AttentionTotal != 1 || len(result.AttentionBatches) != 1 {
		t.Fatalf("concurrent consumption broke the inventory snapshot: %+v, %v", result, err)
	}
	current := f.list(t, nil)
	if current.Items[0].AvailableQuantity != "0.000" || current.Counts.Expiring != 0 ||
		current.Counts.OutOfStock != 1 ||
		current.AttentionTotal != 0 {
		t.Fatalf("committed consumption was not visible to a fresh read: %+v", current)
	}
}
