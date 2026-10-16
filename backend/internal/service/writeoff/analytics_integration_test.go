package writeoff_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	analyticshandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/analytics"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	analyticsrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/analytics"
	analyticsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/analytics"
	"go.uber.org/zap"
)

func (f *fixture) analytics() *analyticsservice.Service {
	return analyticsservice.NewService(
		analyticsrepository.NewRepository(),
		repository.NewRepository(f.pool, zap.NewNop()),
	)
}

func (f *fixture) losses(t *testing.T, filter model.AnalyticsFilter) ([]dto.WriteOffAnalytics, int64) {
	t.Helper()
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 50
	}
	items, total, err := f.analytics().WriteOffs(f.actor(), filter)
	if err != nil {
		t.Fatal(err)
	}
	return items, total
}

func (f *fixture) analyticsRouter() http.Handler {
	r := gin.New()
	r.Use(basehandler.RequestID())
	access := authhandler.NewHandler(f.service)
	api := r.Group("/api/v1", access.RequireRouteAccess)
	basehandler.RegisterIngredientShortageAnalyticsRoutes(api, analyticshandler.NewHandler(f.analytics()))
	basehandler.RegisterWriteOffAnalyticsRoutes(api, analyticshandler.NewHandler(f.analytics()))
	return r
}

func TestWriteOffAnalyticsHistoricalGroupingFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	f.create(t, "1.001", uuid.New())
	f.exec(t, `UPDATE products SET name='Renamed' WHERE id=$1`, f.product)
	f.create(t, "2.002", uuid.New())
	f.exec(t, `UPDATE stock_movements SET occurred_at='2026-01-01T12:00:00Z'`)
	f.exec(
		t,
		`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,occurred_at) VALUES($1,'expiration',-3.003,3.994,'Last historical name','ml','2026-01-01T12:00:01Z')`,
		f.batch,
	)
	f.exec(t, `UPDATE products SET name='Current name',is_active=false WHERE id=$1`, f.product)
	all, total := f.losses(t, model.AnalyticsFilter{})
	want := dto.WriteOffAnalytics{
		ProductID:                 f.product,
		ProductName:               "Last historical name",
		Unit:                      "ml",
		ExpiredWrittenOffQuantity: "3.003",
		ManualWrittenOffQuantity:  "3.003",
		TotalWrittenOffQuantity:   "6.006",
		MovementsCount:            3,
	}
	if total != 1 || len(all) != 1 || all[0] != want {
		t.Fatalf("historical groups: %d %+v", total, all)
	}
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Second)
	missing := uuid.New()
	for _, tc := range []struct {
		f      model.AnalyticsFilter
		n      int64
		amount string
		count  int64
	}{
		{model.AnalyticsFilter{Reason: "all"}, 1, "6.006", 3},
		{model.AnalyticsFilter{Reason: "expiration"}, 1, "3.003", 1},
		{model.AnalyticsFilter{Reason: "manual_writeoff"}, 1, "3.003", 2},
		{model.AnalyticsFilter{DateFrom: &from, DateTo: &to}, 1, "3.003", 2},
		{model.AnalyticsFilter{DateFrom: &to}, 1, "3.003", 1},
		{model.AnalyticsFilter{DateTo: &from}, 0, "", 0},
		{model.AnalyticsFilter{DateFrom: &from, DateTo: &from}, 0, "", 0},
		{model.AnalyticsFilter{ProductID: &missing}, 0, "", 0},
		{model.AnalyticsFilter{ProductID: &f.product, Reason: "manual_writeoff", DateFrom: &from, DateTo: &to}, 1, "3.003", 2},
	} {
		items, n := f.losses(t, tc.f)
		if n > 0 &&
			(tc.f.Reason == "expiration" && items[0].ManualWrittenOffQuantity != "0.000" || tc.f.Reason == "manual_writeoff" && items[0].ExpiredWrittenOffQuantity != "0.000") {
			t.Fatal("zero aggregate format", items)
		}
		if n != tc.n || int64(len(items)) != n ||
			n > 0 && (items[0].TotalWrittenOffQuantity != tc.amount || items[0].MovementsCount != tc.count) {
			t.Fatalf("filter %+v: %d %+v", tc.f, n, items)
		}
	}

	f.exec(
		t,
		`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,occurred_at,note) VALUES($1,'manual_writeoff',-2,0,'Last historical name','pcs','2026-01-01T12:00:02Z','Damage')`,
		f.batch,
	)
	other := uuid.New()
	batch := uuid.New()
	f.exec(
		t,
		`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Another','g','g')`,
		other,
		f.category,
	)
	f.exec(
		t,
		`INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,2,99999999999.999,0,'explicit',clock_timestamp())`,
		batch,
		other,
		f.supply,
	)
	for i := 0; i < 2; i++ {
		f.exec(
			t,
			`INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,occurred_at) VALUES($1,'expiration',-99999999999.999,0,'Last historical name','g',clock_timestamp())`,
			batch,
		)
	}
	all, total = f.losses(t, model.AnalyticsFilter{})
	if total != 3 {
		t.Fatal(total, all)
	}
	for _, item := range all {
		if item.ProductID == other && item.TotalWrittenOffQuantity != "199999999999.998" {
			t.Fatal("aggregate overflow", item)
		}
	}
	for page := 1; page <= 4; page++ {
		items, n := f.losses(t, model.AnalyticsFilter{Page: page, PageSize: 1})
		if n != 3 || page <= 3 && (len(items) != 1 || items[0] != all[page-1]) || page == 4 && len(items) != 0 {
			t.Fatal("unstable page", page, n, items)
		}
	}
}

func TestWriteOffAnalyticsHTTPAccessAndStrictFilters(t *testing.T) {
	f := newFixture(t)
	router := f.analyticsRouter()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "GET", "/analytics/write-offs", "", token, "", "")
		want := 200
		if role == "storekeeper" || role == "order_operator" {
			want = 403
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %d %s", role, w.Code, w.Body)
		}
		if want != 200 {
			continue
		}
		if !strings.Contains(w.Body.String(), `"items":[]`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Body, w.Header())
		}
		for _, query := range []string{"reason=", "reason=receipt", "reason=all&reason=all", "product_id=no", "from=2026-01-01", "to=bad", "from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "page=0", "page_size=101", "page=1&page=2", "sort=name", "search=abc", "bad=%XX"} {
			w = request(router, "GET", "/analytics/write-offs?"+query, "", token, "", "")
			if w.Code != 422 {
				t.Fatalf("%s %d %s", query, w.Code, w.Body)
			}
		}
	}
	if w := request(
		router,
		"GET",
		"/analytics/write-offs",
		"",
		"",
		"",
		"",
	); w.Code != 401 ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
}

type analyticsInterleavingDB struct {
	pgx.Tx
	mutate func()
	once   bool
	t      *testing.T
}

func (q *analyticsInterleavingDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !q.once {
		q.once = true
		var iso, mode string
		if err := q.Tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only')`).
			Scan(&iso, &mode); err != nil {
			q.t.Fatal(err)
		}
		if iso != "repeatable read" || mode != "on" {
			q.t.Fatal(iso, mode)
		}
		q.mutate()
	}
	return q.Tx.Query(ctx, sql, args...)
}

type analyticsInterleavingRepository struct {
	*analyticsrepository.Repository
	mutate func()
	t      *testing.T
}

func (r *analyticsInterleavingRepository) WriteOffs(
	ctx context.Context,
	tx repository.DBTX,
	f model.AnalyticsFilter,
) ([]dto.WriteOffAnalytics, int64, error) {
	return r.Repository.WriteOffs(ctx, &analyticsInterleavingDB{Tx: tx.(pgx.Tx), mutate: r.mutate, t: r.t}, f)
}

func TestWriteOffAnalyticsCountAndPageShareReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	f.create(t, "1", uuid.New())
	before, total := f.losses(t, model.AnalyticsFilter{})
	repo := &analyticsInterleavingRepository{
		Repository: analyticsrepository.NewRepository(),
		mutate:     func() { f.create(t, "2", uuid.New()) },
		t:          t,
	}
	s := analyticsservice.NewService(repo, repository.NewRepository(f.pool, zap.NewNop()))
	got, n, err := s.WriteOffs(f.actor(), model.AnalyticsFilter{Page: 1, PageSize: 50})
	if err != nil || n != total || !reflect.DeepEqual(got, before) {
		t.Fatal("mixed snapshot", got, n, err)
	}
	next, _ := f.losses(t, model.AnalyticsFilter{})
	if next[0].TotalWrittenOffQuantity != "3.000" {
		t.Fatal(next)
	}
}
