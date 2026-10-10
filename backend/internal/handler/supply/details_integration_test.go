package supply_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
)

func (f *listFixture) getDetail(t *testing.T, id uuid.UUID) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet,
		f.server.URL+"/api/v1/supplies/"+id.String(), http.NoBody)
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
	if _, err := uuid.Parse(response.Header.Get("X-Request-ID")); err != nil {
		t.Fatal("request ID missing")
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		t.Fatal("detail response must use application/json")
	}
	if response.StatusCode >= http.StatusBadRequest {
		var result dto.ErrorResponse
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if result.RequestID != response.Header.Get("X-Request-ID") {
			t.Fatal("error request_id does not match X-Request-ID")
		}
	}
	return response.StatusCode, body
}

func TestSupplyDetailsHTTPIntegration(t *testing.T) {
	f := newListFixture(t)
	id := f.ids[1]
	// Offset input and distinct dates catch substitutions between receipt and creation.
	now := time.Now().In(time.FixedZone("MSK", 3*60*60)).Truncate(time.Microsecond)
	created, received, batchCreated := now.Add(-time.Hour), now.Add(-2*time.Hour), now.Add(-30*time.Minute)
	produced := now.Add(-12 * time.Hour)
	if _, err := f.pool.Exec(f.ctx, `UPDATE supplies SET created_at=$2, received_at=$3 WHERE id=$1`,
		id.String(), created, received); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO restaurant_settings
		(id,restaurant_name,expiring_threshold_hours) VALUES (1,'Тест',48)`); err != nil {
		t.Fatal(err)
	}
	var product, category uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `UPDATE products SET is_active=false, standard_shelf_life_hours=720
		RETURNING id,category_id`).Scan(&product, &category); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE batches SET initial_quantity=2.500, remaining_quantity=1.125,
		produced_at=$2, shelf_life_hours=48, expiry_source='calculated', expires_at=$3, created_at=$4
		WHERE supply_id=$1 AND line_number=1`, id.String(), produced, produced.Add(48*time.Hour), batchCreated); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE batches SET expires_at=$2, created_at=$3
		WHERE supply_id=$1 AND line_number=2`, id.String(), now.Add(-time.Hour), batchCreated.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var cream uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO products (category_id,name,unit,display_unit)
		VALUES ($1,'Сливки','ml','l') RETURNING id`, category.String()).Scan(&cream); err != nil {
		t.Fatal(err)
	}
	for _, line := range []struct {
		number  int
		expires time.Time
	}{
		{3, now.Add(60 * time.Hour)}, {4, now.Add(-time.Hour)},
	} {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO batches
			(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)
			VALUES ($1,$2,$3,100,50,'explicit',$4)`, id.String(), cream.String(), line.number, line.expires); err != nil {
			t.Fatal(err)
		}
	}
	status, body := f.getDetail(t, id)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	for _, private := range []string{"fixture-hash", "password", "list@example.test"} {
		if strings.Contains(string(body), private) {
			t.Fatalf("detail exposes %s", private)
		}
	}
	var detail supplydto.SupplyDetailDTO
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	serverTime, err := time.Parse(time.RFC3339Nano, detail.Meta.ServerTime)
	if err != nil || serverTime.Before(now) || serverTime.After(time.Now()) ||
		!strings.HasSuffix(detail.Meta.ServerTime, "Z") {
		t.Fatalf("invalid server time: %s, %v", detail.Meta.ServerTime, err)
	}
	if detail.ID != id || detail.Number != "in-105" || detail.CreatedBy.ID != f.actor ||
		detail.CreatedBy.Name != "Кладовщик" || detail.Status != "received" || detail.ItemsCount != 4 || len(detail.Items) != 4 {
		t.Fatalf("incomplete supply header: %s", body)
	}
	if detail.CreatedAt != created.UTC().Format(time.RFC3339Nano) ||
		detail.ReceivedAt != received.UTC().Format(time.RFC3339Nano) {
		t.Fatal("created_at and received_at are not their independent database values")
	}
	wantStatus := []string{"expiring", "depleted", "valid", "expired"}
	for i := range detail.Items {
		item := &detail.Items[i]
		if item.Batch.LineNumber != i+1 || item.Batch.Status != wantStatus[i] ||
			item.Batch.Supply.ID != id || item.Batch.Supply.Number != detail.Number ||
			item.Batch.Supply.ReceivedAt != detail.ReceivedAt || item.Batch.ProductID != item.Product.ID ||
			item.Batch.Unit != item.Product.Unit {
			t.Fatalf("incorrect line %d: %+v", i+1, item)
		}
		expires, err := time.Parse(time.RFC3339Nano, item.Batch.ExpiresAt)
		if err != nil {
			t.Fatal(err)
		}
		// PostgreSQL dates are nearby here; floor avoids truncating overdue fractions to zero.
		seconds := int64(expires.Sub(serverTime) / time.Second)
		if expires.Before(serverTime) && expires.Sub(serverTime)%time.Second != 0 {
			seconds--
		}
		if item.Batch.RemainingSeconds != seconds {
			t.Fatal("batches did not use meta.server_time for remaining_seconds")
		}
	}
	first := &detail.Items[0]
	if first.Product.ID != product || detail.Items[1].Product.ID != product || first.Product.Name != "Моцарелла" ||
		first.Product.Unit != "g" || first.Product.DisplayUnit != "kg" ||
		first.Batch.InitialQuantity != "2.500" || first.Batch.RemainingQuantity != "1.125" ||
		first.Batch.ExpirySource != "calculated" || first.Batch.ShelfLifeHours == nil || *first.Batch.ShelfLifeHours != 48 ||
		first.Batch.ProducedAt == nil || *first.Batch.ProducedAt != produced.UTC().Format(time.RFC3339Nano) ||
		first.Batch.CreatedAt != batchCreated.UTC().Format(time.RFC3339Nano) {
		t.Fatal("inactive product, duplicate lines, actual expiry or precise quantities were lost")
	}
	if detail.Items[2].Product.ID != cream || detail.Items[2].Product.Name != "Сливки" ||
		detail.Items[2].Product.Unit != "ml" || detail.Items[2].Product.DisplayUnit != "l" ||
		detail.Items[1].Batch.RemainingQuantity != "0.000" || detail.Items[1].Batch.ProducedAt != nil ||
		detail.Items[1].Batch.ShelfLifeHours != nil || !strings.Contains(string(body), `"produced_at":null`) ||
		!strings.Contains(string(body), `"shelf_life_hours":null`) {
		t.Fatal("explicit/depleted batches or second product are incomplete")
	}
}

func TestSupplyDetailsNotFoundIntegration(t *testing.T) {
	f := newListFixture(t)
	var batchID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM batches WHERE supply_id=$1 AND line_number=1`,
		f.ids[1].String()).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		id   uuid.UUID
	}{
		{"unknown UUID", uuid.New()}, {"zero UUID", uuid.Nil}, {"batch UUID is not a supply", batchID},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body := f.getDetail(t, test.id)
			var response dto.ErrorResponse
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if status != http.StatusNotFound || response.Error.Code != "NOT_FOUND" ||
				response.Error.Message != "Ресурс не найден" || response.Error.Details == nil ||
				len(response.Error.Details) != 0 || strings.Contains(string(body), "items") ||
				strings.Contains(string(body), "SELECT") {
				t.Fatalf("missing supply: %d, %s", status, body)
			}
		})
	}
}

func TestSupplyDetailsReadFailureIntegration(t *testing.T) {
	f := newListFixture(t)
	// Fail the second SELECT after a valid supply header was read.
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE products RENAME COLUMN display_unit TO hidden_unit`); err != nil {
		t.Fatal(err)
	}
	status, body := f.getDetail(t, f.ids[1])
	var response dto.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusInternalServerError || response.Error.Code != "INTERNAL_ERROR" ||
		strings.Contains(string(body), "display_unit") || strings.Contains(string(body), "in-105") {
		t.Fatalf("partial detail or SQL exposed on failure: %d, %s", status, body)
	}
}

type detailHookPool struct {
	*pgxpool.Pool
	afterHeader func() error
}

//nolint:gocritic // The signature must match pgxpool.Pool.BeginTx.
func (p *detailHookPool) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &detailHookTx{Tx: tx, afterHeader: p.afterHeader}, nil
}

type detailHookTx struct {
	pgx.Tx
	afterHeader func() error
}

func (t *detailHookTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return &countHookRow{Row: t.Tx.QueryRow(ctx, query, args...), afterCount: t.afterHeader}
}

func TestSupplyDetailsUseSameSnapshotIntegration(t *testing.T) {
	f := newListFixture(t)
	pool := &detailHookPool{Pool: f.pool, afterHeader: func() error {
		if _, err := f.pool.Exec(f.ctx, `UPDATE products SET name='Новое название'`); err != nil {
			return err
		}
		_, err := f.pool.Exec(f.ctx, `UPDATE batches SET remaining_quantity=0`)
		return err
	}}
	repo := repository.NewRepository(pool, zap.NewNop())
	details, err := repo.GetSupplyByID(f.ctx, f.ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if details.ExpiringThresholdHours != 24 || len(details.Items) != 2 ||
		details.Items[0].RemainingQuantity != "1.000" || details.Items[0].ProductName != "Моцарелла" {
		t.Fatal("header and batches did not share a snapshot, or default threshold changed")
	}
	var name, remaining string
	if err := f.pool.QueryRow(f.ctx, `SELECT p.name,b.remaining_quantity::text
		FROM batches b JOIN products p ON p.id=b.product_id
		WHERE b.supply_id=$1 AND b.line_number=1`, f.ids[1].String()).Scan(&name, &remaining); err != nil {
		t.Fatal(err)
	}
	if name != "Новое название" || remaining != "0.000" {
		t.Fatal("concurrent changes did not commit")
	}
}
