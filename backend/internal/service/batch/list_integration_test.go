package batch

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
	batchhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/batch"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	batchrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/batch"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type batchFixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	server   *httptest.Server
	actor    uuid.UUID
	product  uuid.UUID
	token    string
	now      time.Time
	service  *Service
	sequence int
}

func newBatchFixture(t *testing.T) *batchFixture {
	t.Helper()
	dsn := os.Getenv("BATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BATCH_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := "batches_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	f := &batchFixture{
		ctx: ctx, pool: pool, actor: uuid.New(), product: uuid.New(),
		token: uuid.NewString() + uuid.NewString(), now: time.Now().UTC().Truncate(time.Microsecond),
	}
	f.exec(t, `INSERT INTO users (id,name,email,password_hash,role)
		VALUES ($1,'Оператор','batches@example.test','fixture-hash','order_operator')`, f.actor.String())
	f.exec(t, `INSERT INTO sessions (user_id,token_hash,csrf_token,expires_at)
		VALUES ($1,$2,'fixture-csrf',clock_timestamp()+interval '12 hours')`, f.actor.String(), authservice.SessionTokenHash(f.token))
	category := uuid.New()
	f.exec(t, `INSERT INTO product_categories (id,name) VALUES ($1,'Milk')`, category.String())
	f.exec(t, `INSERT INTO products (id,name,category_id,unit,display_unit,is_active)
		VALUES ($1,'Mozzarella',$2,'g','kg',false)`, f.product.String(), category.String())
	logger := zap.NewNop()
	auth, err := authservice.NewService(authrepository.NewRepository(pool), "http://flagman.localhost")
	if err != nil {
		t.Fatal(err)
	}
	f.service = NewService(batchrepository.NewRepository(), repository.NewRepository(pool, logger))
	// Fixed for the lifetime of the server, so expires_at = now can be tested exactly.
	f.service.now = func() time.Time { return f.now }
	router := gin.New()
	router.Use(handler.RequestID(), handler.Recovery(logger))
	handler.RegisterBatchRoutes(
		router.Group("/api/v1"),
		batchhandler.NewHandler(f.service),
		authhandler.NewHandler(auth),
	)
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}

func (f *batchFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *batchFixture) batch(t *testing.T, quantity string, expires time.Time, depleted bool) uuid.UUID {
	t.Helper()
	return f.batchFor(t, f.product, quantity, expires, depleted)
}

func (f *batchFixture) batchFor(
	t *testing.T,
	product uuid.UUID,
	quantity string,
	expires time.Time,
	depleted bool,
) uuid.UUID {
	t.Helper()
	f.sequence++
	id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", f.sequence))
	supply := uuid.New()
	err := repository.NewRepository(f.pool, zap.NewNop()).WithinTx(f.ctx, nil, func(q repository.DBTX) error {
		if _, err := q.Exec(f.ctx, `INSERT INTO supplies (id,number,received_at,created_by)
			VALUES ($1,$2,$3,$4)`, supply.String(), fmt.Sprintf("BATCH-%d", f.sequence), f.now.Add(-72*time.Hour), f.actor.String()); err != nil {
			return err
		}
		if _, err := q.Exec(f.ctx, `INSERT INTO batches
			(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at,created_at)
			VALUES ($1,$2,$3,1,$4::numeric,$4::numeric,'explicit',$5,$6)`, id.String(), supply.String(), product.String(),
			quantity, expires, f.now.Add(-time.Hour)); err != nil {
			return err
		}
		if _, err := q.Exec(f.ctx, `INSERT INTO stock_movements
			(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at)
			SELECT b.id,'receipt',b.initial_quantity,b.initial_quantity,p.name,p.unit,$2,$3
			FROM batches b JOIN products p ON p.id=b.product_id WHERE b.id=$1`, id.String(), f.actor.String(), f.now.Add(-72*time.Hour)); err != nil {
			return err
		}
		if depleted {
			return f.consume(q, id, "0.000")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *batchFixture) consume(q repository.DBTX, id uuid.UUID, remaining string) error {
	if _, err := q.Exec(f.ctx, `INSERT INTO stock_movements
		(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note)
		SELECT b.id,'manual_writeoff',$2::numeric-b.remaining_quantity,$2::numeric,p.name,p.unit,$3,$4,'Test consumption'
		FROM batches b JOIN products p ON p.id=b.product_id WHERE b.id=$1`, id.String(), remaining, f.actor.String(), f.now); err != nil {
		return err
	}
	_, err := q.Exec(f.ctx, `UPDATE batches SET remaining_quantity=$2::numeric WHERE id=$1`, id.String(), remaining)
	return err
}

func (f *batchFixture) request(t *testing.T, id uuid.UUID, query url.Values, cookie bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet,
		f.server.URL+"/api/v1/products/"+id.String()+"/batches?"+query.Encode(), http.NoBody)
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
		if err := json.Unmarshal(body, &result); err != nil || result.RequestID != response.Header.Get("X-Request-ID") {
			t.Fatalf("invalid error envelope: %s (%v)", body, err)
		}
	}
	return response.StatusCode, body
}

func (f *batchFixture) list(t *testing.T, query url.Values) *dto.BatchListResponse {
	t.Helper()
	status, body := f.request(t, f.product, query, true)
	var result dto.BatchListResponse
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("batches: %d %s", status, body)
	}
	if result.Items == nil || result.Meta["server_time"] != f.now.Format(time.RFC3339Nano) {
		t.Fatalf("missing list or inconsistent clock: %s", body)
	}
	return &result
}

func TestBatchListStatusesAndExactExpiryIntegration(t *testing.T) {
	f := newBatchFixture(t)
	tests := []struct {
		quantity string
		expires  time.Time
		depleted bool
		status   string
		seconds  int64
	}{
		{"1.125", f.now.Add(-time.Microsecond), false, "expired", -1},
		{"2.000", f.now, false, "expired", 0},
		{"3.000", f.now.Add(time.Microsecond), false, "expiring", 0},
		{"4.000", f.now.Add(24 * time.Hour), false, "expiring", 86400},
		{"5.000", f.now.Add(24*time.Hour + time.Microsecond), false, "valid", 86400},
		{"6.000", f.now.Add(-time.Hour), true, "depleted", -3600},
		{"7.000", f.now, true, "depleted", 0},
		{"8.000", f.now.Add(time.Hour), true, "depleted", 3600},
		{"9.125", f.now.Add(48 * time.Hour), false, "valid", 172800},
	}
	ids := make([]uuid.UUID, 0, len(tests))
	for _, test := range tests {
		ids = append(ids, f.batch(t, test.quantity, test.expires, test.depleted))
	}
	produced := f.now.Add(-24 * time.Hour)
	f.exec(
		t,
		`UPDATE batches SET produced_at=$2,shelf_life_hours=72,expiry_source='calculated' WHERE id=$1`,
		ids[8].String(),
		produced,
	)
	for _, test := range []struct {
		name    string
		query   url.Values
		indices []int
		total   int64
	}{
		{"default excludes depleted", nil, []int{0, 1, 2, 3, 4, 8}, 6},
		{"all", url.Values{"status": {"all"}}, []int{5, 0, 1, 6, 2, 7, 3, 4, 8}, 9},
		{"valid", url.Values{"status": {"valid"}}, []int{4, 8}, 2},
		{"expiring", url.Values{"status": {"expiring"}}, []int{2, 3}, 2},
		{"expired includes exact now", url.Values{"status": {"expired"}}, []int{0, 1}, 2},
		{"depleted overrides expiry", url.Values{"status": {"depleted"}}, []int{5, 6, 7}, 3},
		{"page", url.Values{"page_size": {"2"}, "page": {"2"}}, []int{2, 3}, 6},
		{"past end", url.Values{"page_size": {"2"}, "page": {"4"}}, []int{}, 6},
		{"safe large offset", url.Values{"page_size": {"1"}, "page": {"9223372036854775807"}}, []int{}, 6},
		{"created_at ties use ID", url.Values{"sort": {"-created_at"}}, []int{0, 1, 2, 3, 4, 8}, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := f.list(t, test.query)
			got := make([]uuid.UUID, 0, len(result.Items))
			want := make([]uuid.UUID, 0, len(test.indices))
			for i := range result.Items {
				got = append(got, result.Items[i].ID)
			}
			for _, index := range test.indices {
				want = append(want, ids[index])
			}
			if !reflect.DeepEqual(got, want) || result.Total != test.total {
				t.Fatalf("IDs %v/%d; want %v/%d", got, result.Total, want, test.total)
			}
		})
	}
	all := f.list(t, url.Values{"status": {"all"}, "sort": {"-created_at"}})
	for i := range all.Items {
		item := &all.Items[i]
		test := &tests[i]
		remaining := test.quantity
		if test.depleted {
			remaining = "0.000"
		}
		if item.Status != test.status || item.RemainingSeconds != test.seconds || item.ExpiresAt != test.expires ||
			item.InitialQuantity != test.quantity || item.RemainingQuantity != remaining || item.ProductID != f.product ||
			item.Unit != "g" || item.LineNumber != 1 || item.CreatedAt != f.now.Add(-time.Hour) ||
			item.Supply.Number != fmt.Sprintf("BATCH-%d", i+1) || item.Supply.ID == uuid.Nil ||
			item.Supply.ReceivedAt != f.now.Add(-72*time.Hour) || item.ExpiresAt.Location() != time.UTC {
			t.Fatalf("batch fields/status mismatch: %+v", item)
		}
		if i == 8 {
			if item.ProducedAt == nil || *item.ProducedAt != produced || item.ShelfLifeHours == nil ||
				*item.ShelfLifeHours != 72 || item.ExpirySource != "calculated" {
				t.Fatal("calculated expiry facts lost")
			}
		} else if item.ProducedAt != nil || item.ShelfLifeHours != nil || item.ExpirySource != "explicit" {
			t.Fatal("nullable explicit expiry facts lost")
		}
	}
	// The threshold changes classification, never the persisted expiry.
	f.exec(t, `INSERT INTO restaurant_settings (restaurant_name,expiring_threshold_hours) VALUES ('Test',48)`)
	result := f.list(t, url.Values{"status": {"expiring"}})
	if result.Total != 4 || len(result.Items) != 4 || result.Items[3].ID != ids[8] ||
		result.Items[3].ExpiresAt != tests[8].expires {
		t.Fatal("configured threshold is not inclusive or rewrote actual expiry")
	}
}

func TestBatchListSupplySortEmptyAndAccessIntegration(t *testing.T) {
	f := newBatchFixture(t)
	if result := f.list(t, nil); result.Total != 0 || len(result.Items) != 0 {
		t.Fatal("existing inactive product without batches must return an empty list")
	}
	first := f.batch(t, "10.125", f.now.Add(time.Hour), false)
	second := f.batch(t, "20.875", f.now.Add(72*time.Hour), false)
	var supply uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT supply_id FROM batches WHERE id=$1`, first.String()).
		Scan(&supply); err != nil {
		t.Fatal(err)
	}
	f.exec(
		t,
		`UPDATE batches SET supply_id=$2,line_number=2,created_at=$3 WHERE id=$1`,
		second.String(),
		supply.String(),
		f.now,
	)
	otherProduct := uuid.New()
	f.exec(t, `INSERT INTO products (id,name,category_id,unit,display_unit)
		SELECT $1,'Other',category_id,unit,display_unit FROM products WHERE id=$2`, otherProduct.String(), f.product.String())
	otherBatch := f.batchFor(t, otherProduct, "1.000", f.now.Add(time.Minute), false)
	f.exec(t, `UPDATE batches SET supply_id=$2,line_number=3 WHERE id=$1`, otherBatch.String(), supply.String())
	result := f.list(t, url.Values{"supply_id": {supply.String()}, "sort": {"-created_at"}})
	if result.Total != 2 || result.Items[0].ID != second || result.Items[0].LineNumber != 2 ||
		result.Items[1].ID != first {
		t.Fatal("supply filter or descending creation order is incorrect")
	}
	result = f.list(t, url.Values{"supply_id": {supply.String()}, "status": {"expiring"}})
	if result.Total != 1 || result.Items[0].ID != first {
		t.Fatal("status and supply filters must intersect")
	}
	if result := f.list(t, url.Values{"supply_id": {uuid.NewString()}}); result.Total != 0 || len(result.Items) != 0 {
		t.Fatal("unknown supply filter must return an empty list")
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor.String(), role)
		status, body := f.request(t, f.product, nil, true)
		if status != http.StatusOK {
			t.Fatalf("role %s: %d %s", role, status, body)
		}
		for _, secret := range []string{"fixture-hash", "fixture-csrf", f.token, "password", "token_hash"} {
			if strings.Contains(string(body), secret) {
				t.Fatal("batch projection exposes secrets")
			}
		}
	}
	if status, _ := f.request(t, uuid.New(), nil, true); status != http.StatusNotFound {
		t.Fatal("unknown product must return 404")
	}
	if status, _ := f.request(t, f.product, nil, false); status != http.StatusUnauthorized {
		t.Fatal("missing session accepted")
	}
	f.exec(t, `UPDATE sessions SET revoked_at=clock_timestamp()`)
	if status, _ := f.request(t, f.product, nil, true); status != http.StatusUnauthorized {
		t.Fatal("revoked session accepted")
	}
	f.exec(t, `UPDATE sessions SET revoked_at=NULL`)
	f.exec(t, `ALTER TABLE batches RENAME COLUMN initial_quantity TO hidden_quantity`)
	status, body := f.request(t, f.product, nil, true)
	if status != http.StatusInternalServerError || !strings.Contains(string(body), "INTERNAL_ERROR") ||
		strings.Contains(string(body), "BATCH-1") || strings.Contains(string(body), "hidden_quantity") {
		t.Fatalf("late SQL failure leaked partial data: %d %s", status, body)
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
	if strings.Contains(query, "SELECT COUNT(*) FROM selected") {
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

func TestBatchListConsumptionAndConsistentSnapshotIntegration(t *testing.T) {
	f := newBatchFixture(t)
	first := f.batch(t, "10.125", f.now.Add(time.Hour), false)
	second := f.batch(t, "2.000", f.now.Add(72*time.Hour), false)
	transactions := repository.NewRepository(f.pool, zap.NewNop())
	if err := transactions.WithinTx(
		f.ctx,
		nil,
		func(q repository.DBTX) error { return f.consume(q, first, "2.125") },
	); err != nil {
		t.Fatal(err)
	}
	partial := f.list(t, nil)
	if partial.Total != 2 || partial.Items[0].InitialQuantity != "10.125" ||
		partial.Items[0].RemainingQuantity != "2.125" ||
		partial.Items[0].Status != "expiring" {
		t.Fatal("partial consumption did not preserve batch facts")
	}
	pool := &countHookPool{Pool: f.pool, afterCount: func() error {
		return transactions.WithinTx(f.ctx, nil, func(q repository.DBTX) error { return f.consume(q, first, "0.000") })
	}}
	s := NewService(batchrepository.NewRepository(), repository.NewRepository(pool, zap.NewNop()))
	s.now = func() time.Time { return f.now }
	result, err := s.ListBatches(
		f.ctx,
		model.BatchFilter{ProductID: f.product, Page: 1, PageSize: 50, Sort: "expires_at"},
	)
	if err != nil || result.Total != 2 || len(result.Items) != 2 || result.Items[0].RemainingQuantity != "2.125" {
		t.Fatalf("concurrent consumption broke count/page snapshot: %+v, %v", result, err)
	}
	current := f.list(t, nil)
	if current.Total != 1 || len(current.Items) != 1 || current.Items[0].ID != second {
		t.Fatal("fresh default list did not exclude newly depleted batch")
	}
	depleted := f.list(t, url.Values{"status": {"depleted"}})
	if depleted.Total != 1 || depleted.Items[0].ID != first || depleted.Items[0].Status != "depleted" ||
		depleted.Items[0].RemainingQuantity != "0.000" || depleted.Items[0].RemainingSeconds != 3600 {
		t.Fatal("depleted filter lost the batch or its signed deadline")
	}
	var inconsistent int
	if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM batches b WHERE b.remaining_quantity !=
		(SELECT SUM(delta_quantity) FROM stock_movements WHERE batch_id=b.id)`).Scan(&inconsistent); err != nil || inconsistent != 0 {
		t.Fatalf("fixture journal mismatch: %d, %v", inconsistent, err)
	}
}
