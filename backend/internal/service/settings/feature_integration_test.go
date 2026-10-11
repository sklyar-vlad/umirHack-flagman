package settings_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	dashboardrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	offersrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	dashboardservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dashboard"
	offersservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/hotoffers"
	"go.uber.org/zap"
)

func TestAutoWriteOffCanBeEnabledAndDisabledThroughSettings(t *testing.T) {
	f := newFixture(t)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	for i, enabled := range []bool{true, false} {
		raw, _ := json.Marshal(map[string]any{"auto_writeoff_expired": enabled, "expected_revision": i + 1})
		w := request(f.router(), "PATCH", "/settings", string(raw), token, session.CSRFToken, "https://flagman.example")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		after, err := f.settings.Get(f.actor())
		if err != nil || after.AutoWriteOffExpired != enabled || after.Revision != i+2 {
			t.Fatal(after, err)
		}
	}
	var audits, events int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events)`).Scan(&audits, &events); err != nil || audits != 2 || events != 2 {
		t.Fatal(audits, events, err)
	}
}
func TestSettingsChangesAppendSafeInvalidationOutbox(t *testing.T) {
	f := newFixture(t)
	ctx := requestctx.WithID(f.actor(), "settings-change")
	for _, input := range []dto.UpdateSettingsRequest{{RestaurantName: ptr("Name"), ExpectedRevision: 1}, {Timezone: ptr("UTC"), ExpectedRevision: 2}, {ExpiringThresholdHours: ptr(48), ExpectedRevision: 3}, {AutoWriteOffExpired: ptr(false), ExpectedRevision: 4}} {
		if _, err := f.settings.Update(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := f.pool.Query(ctx, `SELECT event_type,actor_id,payload FROM outbox_events ORDER BY cursor`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var typ string
		var actor uuid.UUID
		var raw []byte
		if err := rows.Scan(&typ, &actor, &raw); err != nil {
			t.Fatal(err)
		}
		var p model.OutboxPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		if typ != "settings.changed" || actor != f.user || p.RequestID != "settings-change" || p.OccurredAt.IsZero() {
			t.Fatal("outbox metadata")
		}
		if !slices.Equal(p.Topics, []model.EventTopic{model.TopicInventory, model.TopicBatches, model.TopicDashboard, model.TopicDishes, model.TopicStopList, model.TopicHotOffers, model.TopicProcurementRecommendations, model.TopicSettings}) {
			t.Fatal("missing invalidation topics", p.Topics)
		}
		if len(p.ProductIDs)+len(p.BatchIDs)+len(p.DishIDs)+len(p.CategoryIDs)+len(p.SupplyIDs)+len(p.OrderIDs)+len(p.UserIDs) != 0 {
			t.Fatal("unrelated entity identifiers")
		}
		count++
	}
	if err := rows.Err(); err != nil || count != 4 {
		t.Fatal("outbox count", count, err)
	}
}
func TestSettingsRecalculateRecommendationsAndShellWithoutRewritingStock(t *testing.T) {
	f := newFixture(t)
	category, product, supply, batch, dish, recipe := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Category')`, category)
	f.exec(t, `INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,'Product','g','g')`, product, category)
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by) VALUES($1,'Supply',$2,$3)`, supply, now.Add(-time.Hour), f.user)
	f.exec(t, `INSERT INTO batches(id,product_id,supply_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,10,10,'explicit',$4)`, batch, product, supply, now.Add(36*time.Hour))
	f.exec(t, `INSERT INTO dishes(id,name) VALUES($1,'Dish')`, dish)
	f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, recipe, dish, f.user)
	f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,1)`, recipe, product)
	base := repository.NewRepository(f.pool, zap.NewNop())
	offers := offersservice.NewService(offersrepo.NewRepository(), menurepo.NewRepository(), base)
	dashboard := dashboardservice.NewService(dashboardrepo.NewRepository(), base)
	filter := model.HotOffersFilter{Page: 1, PageSize: 50}
	first, err := offers.List(f.actor(), filter)
	if err != nil || first.Total != 0 {
		t.Fatal("outside threshold", first.Total, err)
	}
	firstDashboard, err := dashboard.Get(f.actor(), "admin")
	if err != nil || firstDashboard.ExpiringBatchesCount != 0 {
		t.Fatal("dashboard outside threshold", err)
	}
	_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.settings.Update(f.actor(), dto.UpdateSettingsRequest{RestaurantName: ptr("New restaurant"), Timezone: ptr("Asia/Tokyo"), ExpiringThresholdHours: ptr(48), ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	next, err := offers.List(f.actor(), filter)
	if err != nil || next.Total != 1 || next.Items[0].Dish.ID != dish {
		t.Fatal("threshold did not recalculate", next.Total, err)
	}
	nextDashboard, err := dashboard.Get(f.actor(), "admin")
	if err != nil || nextDashboard.ExpiringBatchesCount != 1 || len(nextDashboard.HotOffers) != 1 || nextDashboard.Settings.RestaurantName != "New restaurant" || nextDashboard.Settings.Timezone != "Asia/Tokyo" || nextDashboard.Settings.ExpiringThresholdHours != 48 {
		t.Fatal("dashboard did not recalculate", err)
	}
	session, err := f.service.CurrentSession(f.ctx, token)
	if err != nil || session.Settings.RestaurantName != "New restaurant" || session.Settings.Timezone != "Asia/Tokyo" || session.Settings.ExpiringThresholdHours != 48 {
		t.Fatal("stale shell settings", err)
	}
	var remaining string
	var expires time.Time
	var movements int
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text,expires_at,(SELECT count(*) FROM stock_movements) FROM batches WHERE id=$1`, batch).Scan(&remaining, &expires, &movements); err != nil || remaining != "10.000" || !expires.Equal(now.Add(36*time.Hour).Truncate(time.Microsecond)) || movements != 0 {
		t.Fatal("settings rewrote stock", remaining, expires, movements, err)
	}
	_, err = f.settings.Update(f.actor(), dto.UpdateSettingsRequest{ExpiringThresholdHours: ptr(24), ExpectedRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	last, err := offers.List(f.actor(), filter)
	if err != nil || last.Total != 0 {
		t.Fatal("lower threshold did not recalculate", last.Total, err)
	}
}
