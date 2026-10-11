package writeoff_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	writeoffrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"go.uber.org/zap"
)

func (f *fixture) list(t *testing.T, filter model.StockMovementFilter) ([]dto.StockMovement, int64) {
	t.Helper()
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 50
	}
	items, total, err := f.movements.List(f.actor(), filter)
	if err != nil {
		t.Fatal(err)
	}
	return items, total
}
func TestMovementHistorySnapshotsFiltersAndStablePages(t *testing.T) {
	f := newFixture(t)
	r := f.create(t, "1", uuid.New())
	var m dto.StockMovement
	_ = json.Unmarshal(r.Body, &m)
	f.exec(t, `UPDATE products SET name='Renamed' WHERE id=$1`, f.product)
	f.create(t, "2", uuid.New())
	f.exec(t, `UPDATE stock_movements SET occurred_at='2026-01-01T12:00:00Z'`)
	all, total := f.list(t, model.StockMovementFilter{})
	if total != 3 || len(all) != 3 {
		t.Fatal("all", all, total)
	}
	found := false
	for _, v := range all {
		if v.ID == m.ID {
			found = true
			if v.ProductNameSnapshot != "Молоко" || v.UnitSnapshot != "ml" || v.RemainingQuantityAfter != "9.000" || *v.Note != "Порча продукта" || v.Actor.ID != f.user || v.OrderID != nil || v.OrderItemID != nil {
				t.Fatal("historical", v)
			}
		}
	}
	if !found {
		t.Fatal("movement missing")
	}
	seen := map[uuid.UUID]bool{}
	for page := 1; page <= 4; page++ {
		items, n := f.list(t, model.StockMovementFilter{Page: page, PageSize: 1})
		if n != 3 {
			t.Fatal("total")
		}
		if page == 4 {
			if len(items) != 0 {
				t.Fatal("empty")
			}
			continue
		}
		if len(items) != 1 || seen[items[0].ID] || items[0].ID != all[page-1].ID {
			t.Fatal("unstable page", items)
		}
		seen[items[0].ID] = true
	}
	missing := uuid.New()
	same := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	later := same.Add(time.Microsecond)
	for _, test := range []struct {
		f    model.StockMovementFilter
		want int64
	}{
		{model.StockMovementFilter{ProductID: &f.product}, 3}, {model.StockMovementFilter{BatchID: &f.batch}, 3},
		{model.StockMovementFilter{OrderID: &missing}, 0}, {model.StockMovementFilter{ProductID: &missing}, 0}, {model.StockMovementFilter{BatchID: &missing}, 0},
		{model.StockMovementFilter{Type: "manual_writeoff"}, 2}, {model.StockMovementFilter{Type: "receipt"}, 1},
		{model.StockMovementFilter{Type: "expiration"}, 0}, {model.StockMovementFilter{DateFrom: &same, DateTo: &later}, 3},
		{model.StockMovementFilter{DateTo: &same}, 0}, {model.StockMovementFilter{DateFrom: &same, DateTo: &same}, 0},
		{model.StockMovementFilter{ProductID: &f.product, BatchID: &f.batch, Type: "manual_writeoff", DateFrom: &same, DateTo: &later}, 2},
	} {
		items, n := f.list(t, test.f)
		if n != test.want || int64(len(items)) != n {
			t.Fatalf("filter %+v: %d %v", test.f, n, items)
		}
	}
	// System expiration actors are represented by null, with no user secrets.
	f.exec(t, `INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,occurred_at) VALUES($1,'expiration',-7,0,'Old name','ml',clock_timestamp())`, f.batch)
	items, n := f.list(t, model.StockMovementFilter{Type: "expiration"})
	if n != 1 || items[0].Actor != nil || items[0].ProductNameSnapshot != "Old name" {
		t.Fatal(items, n)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), "csrf") || strings.Contains(string(raw), "token") {
		t.Fatal("unsafe projection")
	}
}
func TestMovementHistoryHTTPRolesAndInvalidFilters(t *testing.T) {
	f := newFixture(t)
	router := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "GET", "/stock-movements", "", token, "", "")
		want := 200
		if role == "order_operator" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("%s %d %s", role, w.Code, w.Body)
		}
		if role == "order_operator" {
			continue
		}
		for _, query := range []string{"type=unknown", "type=", "type=receipt&type=receipt", "sort=occurred_at", "product_id=", "batch_id=no", "order_id=no", "from=2026-01-01", "to=bad", "from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "page=0", "page_size=101", "search=abc", "bad=%XX"} {
			w = request(router, "GET", "/stock-movements?"+query, "", token, session.CSRFToken, "")
			if w.Code != 422 {
				t.Fatalf("%s %d %s", query, w.Code, w.Body)
			}
		}
	}
	w := request(router, "GET", "/stock-movements", "", "", "", "")
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

type interleavingDB struct {
	pgx.Tx
	f    *fixture
	t    *testing.T
	once bool
}

func (q *interleavingDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !q.once {
		q.once = true
		var iso, mode string
		if err := q.Tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only')`).Scan(&iso, &mode); err != nil {
			q.t.Fatal(err)
		}
		if iso != "repeatable read" || mode != "on" {
			q.t.Fatal(iso, mode)
		}
		q.f.create(q.t, "1", uuid.New())
	}
	return q.Tx.Query(ctx, sql, args...)
}

type interleavingRepository struct {
	*writeoffrepository.Repository
	f *fixture
	t *testing.T
}

func (r *interleavingRepository) List(ctx context.Context, tx repository.DBTX, filter model.StockMovementFilter) ([]dto.StockMovement, int64, error) {
	return r.Repository.List(ctx, &interleavingDB{Tx: tx.(pgx.Tx), f: r.f, t: r.t}, filter)
}
func TestMovementHistoryCountAndPageShareReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	before, total := f.list(t, model.StockMovementFilter{})
	base := repository.NewRepository(f.pool, zap.NewNop())
	service := writeoffservice.NewHistoryService(&interleavingRepository{writeoffrepository.NewRepository(), f, t}, base)
	got, n, err := service.List(f.actor(), model.StockMovementFilter{Page: 1, PageSize: 50})
	if err != nil || n != total || !reflect.DeepEqual(before, got) {
		t.Fatalf("mixed snapshot %d %v %v", n, got, err)
	}
	_, next := f.list(t, model.StockMovementFilter{})
	if next != total+1 || f.balance(t) != "9.000" {
		t.Fatal("concurrent write missing")
	}
}
