package forecast

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	forecasthandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/forecast"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	forecastrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/forecast"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

func (f *fixture) service() *Service {
	s := NewService(
		forecastrepo.NewRepository(),
		menurepo.NewRepository(),
		repository.NewRepository(f.pool, zap.NewNop()),
	)
	s.now = func() time.Time { return f.now }
	return s
}

func filter() model.ForecastFilter {
	return model.ForecastFilter{Page: 1, PageSize: 50, LookbackDays: 7, HorizonHours: 72}
}

func (f *fixture) forecast(t *testing.T) dto.StopListForecast {
	t.Helper()
	r, err := f.service().List(f.actor(), filter())
	if err != nil || r.Total != 1 || len(r.Items) != 1 {
		t.Fatal(r, err)
	}
	return r.Items[0]
}

func TestForecastConfidenceAssumptionsAndContractCodes(t *testing.T) {
	f := newFixture(t)
	i := f.forecast(t)
	if i.EstimatedStopAt != nil || i.LimitingProductID != nil || i.Cause != "unknown" ||
		i.Confidence != "insufficient_data" ||
		!slices.Contains(i.Assumptions, "missing_history:"+f.product.String()) ||
		!slices.Contains(i.Assumptions, "unknown_demand_no_expiry_stop_within_horizon") {
		t.Fatal(i)
	}
	// Wrong historical units cannot turn unknown demand into a precise prediction.
	f.consumption(t, "700", "ml", f.now.Add(-time.Hour))
	if got := f.forecast(t); got.Confidence != "insufficient_data" || got.EstimatedStopAt != nil {
		t.Fatal(got)
	}
	f.consumption(t, "168", "g", f.now.Add(-time.Hour))
	i = f.forecast(t)
	if i.EstimatedStopAt != nil || i.Cause != "unknown" || i.Confidence != "low" ||
		!slices.Contains(i.Assumptions, "no_stop_within_horizon") {
		t.Fatal(i)
	}
	f.exec(t, `UPDATE batches SET expires_at=$2 WHERE id=$1`, f.batch, f.now.Add(2*time.Hour))
	i = f.forecast(t)
	if i.EstimatedStopAt == nil || !i.EstimatedStopAt.Equal(f.now.Add(2*time.Hour)) || i.Cause != "expiry" ||
		i.LimitingProductID == nil ||
		*i.LimitingProductID != f.product ||
		i.Confidence != "low" {
		t.Fatal(i)
	}
	f.exec(t, `UPDATE batches SET remaining_quantity=5 WHERE id=$1`, f.batch)
	i = f.forecast(t)
	if i.Cause != "already_unavailable" || !i.EstimatedStopAt.Equal(f.now) {
		t.Fatal(i)
	}
	// The service protects direct callers before starting transactions.
	if _, err := f.service().List(context.Background(), filter()); err == nil {
		t.Fatal("missing actor accepted")
	}
	if _, err := f.service().
		List(requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: "storekeeper"}), filter()); err == nil {
		t.Fatal("role accepted")
	}
}

func TestForecastHTTPRolesStrictQueryAndListEnvelope(t *testing.T) {
	f := newFixture(t)
	password := "forecast integration password"
	hash, err := authservice.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE users SET password_hash=$2 WHERE id=$1`, f.user, hash)
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name) VALUES('Test Restaurant')`)
	auth, err := authservice.NewService(authrepo.NewRepository(f.pool), "https://flagman.example")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(basehandler.RequestID())
	access := authhandler.NewHandler(auth)
	api := r.Group("/api/v1", access.RequireRouteAccess)
	basehandler.RegisterStopListForecastRoutes(api, forecasthandler.NewHandler(f.service()))
	request := func(query, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/stop-list-forecast"+query, http.NoBody)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "flagman_session", Value: token})
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := request("", ""); w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body)
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		_, token, err := auth.Authenticate(f.ctx, "manager@example.test", password)
		if err != nil {
			t.Fatal(err)
		}
		w := request("", token)
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
		var data dto.ListResponse[dto.StopListForecast]
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Total != 1 || data.Page != 1 || data.PageSize != 50 || data.Meta["lookback_days"] != float64(7) ||
			data.Meta["horizon_hours"] != float64(72) ||
			data.Items[0].Confidence != "insufficient_data" {
			t.Fatal(data)
		}
		for _, q := range []string{"?lookback_days=0", "?lookback_days=91", "?horizon_hours=0", "?horizon_hours=721", "?lookback_days=", "?horizon_hours=1.5", "?page=0", "?page_size=101", "?lookback_days=7&lookback_days=7", "?horizon_hours=72&horizon_hours=72", "?bad=%XX", "?sort=id", "?from=2026-01-01"} {
			if w := request(q, token); w.Code != 422 {
				t.Fatal(q, w.Code, w.Body)
			}
		}
		w = request("?lookback_days=90&horizon_hours=720&page=2&page_size=1", token)
		if err := json.Unmarshal(
			w.Body.Bytes(),
			&data,
		); err != nil || w.Code != 200 || len(data.Items) != 0 ||
			data.Total != 1 {
			t.Fatal(w.Code, w.Body, err)
		}
		f.exec(t, `UPDATE dishes SET is_active=false WHERE id=$1`, f.dish)
		w = request("", token)
		if err := json.Unmarshal(
			w.Body.Bytes(),
			&data,
		); err != nil || data.Items == nil || len(data.Items) != 0 ||
			data.Total != 0 {
			t.Fatal(w.Body, err)
		}
		f.exec(t, `UPDATE dishes SET is_active=true WHERE id=$1`, f.dish)
	}
}

func TestForecastSortBeforePaginationWithStableTiesAndNullLast(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `UPDATE batches SET expires_at=$2 WHERE id=$1`, f.batch, f.now.Add(time.Hour))
	ids := []uuid.UUID{f.dish}
	for _, id := range []uuid.UUID{uuid.MustParse("00000000-0000-4000-8000-000000000001"), uuid.MustParse("00000000-0000-4000-8000-000000000002")} {
		rv := uuid.New()
		f.exec(t, `INSERT INTO dishes(id,name) VALUES($1,$2)`, id, id.String())
		f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, rv, id, f.user)
		f.exec(
			t,
			`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,10)`,
			rv,
			f.product,
		)
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	for p := 1; p <= 4; p++ {
		q := filter()
		q.Page = p
		q.PageSize = 1
		r, err := f.service().List(f.actor(), q)
		if err != nil || r.Total != 3 || p <= 3 && (len(r.Items) != 1 || r.Items[0].DishID != ids[p-1]) ||
			p == 4 && len(r.Items) != 0 {
			t.Fatal(p, r, err)
		}
	}
	// Null estimates come after dated estimates regardless of their ID.
	dishes := []model.Dish{
		{
			ID:       uuid.MustParse("00000000-0000-4000-8000-000000000001"),
			IsActive: true,
			Recipe: []model.RecipeIngredient{
				{
					ProductID:          uuid.New(),
					Unit:               "g",
					QuantityPerPortion: "1",
					AvailableQuantity:  "10",
					ExpiredQuantity:    "0",
				},
			},
		},
		{ID: uuid.New(), IsActive: true},
	}
	items, err := BuildForecasts(
		dishes,
		[]model.OrderBatch{
			{ProductID: dishes[0].Recipe[0].ProductID, RemainingQuantity: "10", ExpiresAt: f.now.Add(100 * time.Hour)},
		},
		nil,
		f.now,
		filter(),
	)
	if err != nil || items[0].DishID != dishes[1].ID || items[1].EstimatedStopAt != nil {
		t.Fatal(items, err)
	}
}
