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
	analyticsrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/analytics"
	analyticsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/analytics"
	"go.uber.org/zap"
)

func (f *fixture) shortages(t *testing.T, filter model.AnalyticsFilter) ([]dto.IngredientShortageAnalytics, int64) {
	t.Helper()
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 50
	}
	items, n, err := f.analytics().IngredientShortages(f.actor(), filter)
	if err != nil {
		t.Fatal(err)
	}
	return items, n
}

func (f *fixture) attempt(t *testing.T, id uuid.UUID, at time.Time, code string, shortages ...dto.Shortage) {
	t.Helper()
	if shortages == nil {
		shortages = []dto.Shortage{}
	}
	raw, err := json.Marshal(shortages)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		t,
		`INSERT INTO order_attempts(id,created_by,created_at,reason_code,items_snapshot,shortages,issues) VALUES($1,$2,$3,$4,'[]',$5,'[]')`,
		id,
		f.user,
		at,
		code,
		raw,
	)
}

func TestIngredientShortageAnalyticsHistoryExactSumsFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Second)

	other := uuid.New()
	first := dto.Shortage{ProductID: f.product, ProductName: "Old milk", Unit: "ml", MissingQuantity: "1.001"}
	second := first
	second.ProductName = "New milk"
	second.MissingQuantity = "2.002"
	pcs := dto.Shortage{ProductID: other, ProductName: "Egg", Unit: "pcs", MissingQuantity: "2.000"}
	f.attempt(t, uuid.New(), from, "INSUFFICIENT_STOCK", first, pcs)
	f.attempt(t, uuid.New(), to, "INSUFFICIENT_STOCK", second)
	for _, code := range []string{"RECIPE_CHANGED", "DISH_UNAVAILABLE"} {
		f.attempt(t, uuid.New(), to, code, second, pcs)
	}
	f.attempt(t, uuid.New(), to, "INSUFFICIENT_STOCK")
	f.exec(t, `UPDATE products SET name='Current',unit='g',display_unit='g',is_active=false WHERE id=$1`, f.product)
	all, n := f.shortages(t, model.AnalyticsFilter{})
	if n != 2 || len(all) != 2 ||
		all[0] != (dto.IngredientShortageAnalytics{ProductID: f.product, ProductName: "New milk", Unit: "ml", RejectedAttemptsCount: 2, TotalMissingQuantity: "3.003"}) {
		t.Fatal(n, all)
	}
	if all[1].ProductID != other || all[1].TotalMissingQuantity != "2.000" || all[1].RejectedAttemptsCount != 1 {
		t.Fatal(all)
	}
	for _, tc := range []struct {
		filter   model.AnalyticsFilter
		groups   int64
		quantity string
		count    int64
	}{
		{model.AnalyticsFilter{DateFrom: &from, DateTo: &to}, 2, "1.001", 1},
		{model.AnalyticsFilter{DateFrom: &to}, 1, "2.002", 1},
		{model.AnalyticsFilter{DateTo: &from}, 0, "", 0},
		{model.AnalyticsFilter{DateFrom: &from, DateTo: &from}, 0, "", 0},
	} {
		items, n := f.shortages(t, tc.filter)
		if n != tc.groups || int64(len(items)) != n {
			t.Fatal(tc, items, n)
		}
		for _, item := range items {
			if item.ProductID == f.product &&
				(item.TotalMissingQuantity != tc.quantity || item.RejectedAttemptsCount != tc.count) {
				t.Fatal(tc, item)
			}
		}
	}

	pcs.ProductID = f.product
	f.attempt(t, uuid.New(), to.Add(time.Second), "INSUFFICIENT_STOCK", pcs)
	large := first
	large.MissingQuantity = "199999999999.998"
	f.attempt(t, uuid.New(), to.Add(2*time.Second), "INSUFFICIENT_STOCK", large)
	all, n = f.shortages(t, model.AnalyticsFilter{})
	if n != 3 || all[0].TotalMissingQuantity != "200000000003.001" || all[0].RejectedAttemptsCount != 3 {
		t.Fatal("overflow or mixed units", n, all)
	}
	for page := 1; page <= 4; page++ {
		items, n := f.shortages(t, model.AnalyticsFilter{Page: page, PageSize: 1})
		if n != 3 || page <= 3 && (len(items) != 1 || items[0] != all[page-1]) || page == 4 && len(items) != 0 {
			t.Fatal("unstable", page, n, items)
		}
	}
}

func TestIngredientShortageAnalyticsHTTPRolesAndStrictFilters(t *testing.T) {
	f := newFixture(t)
	router := f.analyticsRouter()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "GET", "/analytics/ingredient-shortages", "", token, "", "")
		want := 200
		if role == "storekeeper" || role == "order_operator" {
			want = 403
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(role, w.Code, w.Body)
		}
		if want != 200 {
			continue
		}
		if !strings.Contains(w.Body.String(), `"items":[]`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Body, w.Header())
		}
		for _, query := range []string{"sort=-rejected_attempts_count", "product_id=" + f.product.String(), "reason=all", "search=abc", "from=bad", "to=2026-01-01", "from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "page=0", "page_size=101", "from=bad&from=bad", "bad=%XX"} {
			w = request(router, "GET", "/analytics/ingredient-shortages?"+query, "", token, "", "")
			if w.Code != 422 {
				t.Fatal(query, w.Code, w.Body)
			}
		}
	}
	if w := request(
		router,
		"GET",
		"/analytics/ingredient-shortages",
		"",
		"",
		"",
		"",
	); w.Code != 401 ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
}

func (r *analyticsInterleavingRepository) IngredientShortages(
	ctx context.Context,
	tx repository.DBTX,
	f model.AnalyticsFilter,
) ([]dto.IngredientShortageAnalytics, int64, error) {
	return r.Repository.IngredientShortages(ctx, &analyticsInterleavingDB{Tx: tx.(pgx.Tx), mutate: r.mutate, t: r.t}, f)
}

func TestIngredientShortageAnalyticsCountAndPageShareReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	fact := dto.Shortage{ProductID: f.product, ProductName: "Milk", Unit: "ml", MissingQuantity: "1.001"}
	f.attempt(t, uuid.New(), time.Now(), "INSUFFICIENT_STOCK", fact)
	before, total := f.shortages(t, model.AnalyticsFilter{})
	mutate := func() { fact.ProductID = uuid.New(); f.attempt(t, uuid.New(), time.Now(), "INSUFFICIENT_STOCK", fact) }
	repo := &analyticsInterleavingRepository{Repository: analyticsrepository.NewRepository(), mutate: mutate, t: t}
	s := analyticsservice.NewService(repo, repository.NewRepository(f.pool, zap.NewNop()))
	got, n, err := s.IngredientShortages(f.actor(), model.AnalyticsFilter{Page: 1, PageSize: 50})
	if err != nil || n != total || !reflect.DeepEqual(got, before) {
		t.Fatal("mixed snapshot", got, n, err)
	}
	_, next := f.shortages(t, model.AnalyticsFilter{})
	if next != total+1 {
		t.Fatal("insert not committed")
	}
}
