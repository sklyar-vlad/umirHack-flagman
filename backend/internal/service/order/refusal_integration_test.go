package order

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
)

func orderWriteSnapshot(t *testing.T, f *creationFixture) map[string]any {
	t.Helper()
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(f.snapshot(t)), &snapshot); err != nil {
		t.Fatal(err)
	}
	delete(snapshot, "attempts")
	delete(snapshot, "idempotency")
	return snapshot
}

func TestOrderRefusalIntegrationFactsAndIdempotentReplay(t *testing.T) {
	for _, scenario := range []struct {
		name, code     string
		active, recipe bool
		version        int
	}{
		{"shortage", "INSUFFICIENT_STOCK", true, true, 1},
		{"changed", "RECIPE_CHANGED", true, true, 2},
		{"inactive", "DISH_UNAVAILABLE", false, true, 1},
		{"missing", "DISH_UNAVAILABLE", true, false, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newCreationFixture(t)
			product, dish := f.product(t, "Ingredient"), f.dish(t, "Snapshot dish", scenario.active)
			f.batch(t, product, "1.000", f.now.Add(time.Hour))
			if scenario.recipe {
				f.recipe(t, dish, scenario.version, map[uuid.UUID]string{product: "1.000"})
			}
			version := 1
			body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 2, RecipeVersion: &version})
			key := uuid.New()
			before := orderWriteSnapshot(t, f)
			metricBefore := testutil.ToFloat64(telemetry.Default.Refusals.WithLabelValues(scenario.code))
			response := f.createRequest(body, key)
			if response.Code != http.StatusConflict || response.Header().Get("Location") != "" {
				t.Fatalf("refusal: %d %s", response.Code, response.Body.String())
			}
			var refusal struct {
				Error struct {
					Code    string `json:"code"`
					Details struct {
						AttemptID    uuid.UUID         `json:"attempt_id"`
						Requirements []dto.Requirement `json:"requirements"`
						Shortages    []dto.Shortage    `json:"shortages"`
						Issues       []dto.OrderIssue  `json:"issues"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil {
				t.Fatal(err)
			}
			if refusal.Error.Code != scenario.code || refusal.Error.Details.AttemptID == uuid.Nil ||
				refusal.Error.Details.Requirements == nil ||
				refusal.Error.Details.Shortages == nil ||
				refusal.Error.Details.Issues == nil {
				t.Fatalf("refusal body: %s", response.Body.String())
			}
			if scenario.name == "shortage" &&
				(len(refusal.Error.Details.Requirements) != 1 || refusal.Error.Details.Requirements[0].RequiredQuantity != "2.000" || refusal.Error.Details.Requirements[0].MissingQuantity != "1.000") {
				t.Fatal("incorrect shortage demand")
			}
			var itemsData, shortagesData, issuesData []byte
			var code string
			var actor uuid.UUID
			var created time.Time
			if err := f.pool.QueryRow(f.ctx, `SELECT created_by,created_at,reason_code,items_snapshot,shortages,issues FROM order_attempts WHERE id=$1`, refusal.Error.Details.AttemptID).
				Scan(&actor, &created, &code, &itemsData, &shortagesData, &issuesData); err != nil {
				t.Fatal(err)
			}
			var items []dto.OrderItemSnapshot
			var shortages []dto.Shortage
			var issues []dto.OrderIssue
			if err := json.Unmarshal(itemsData, &items); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(shortagesData, &shortages); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(issuesData, &issues); err != nil {
				t.Fatal(err)
			}
			if actor != f.actor || !created.Equal(f.now) || code != scenario.code || len(items) != 1 ||
				items[0].DishName != "Snapshot dish" ||
				items[0].Portions != 2 ||
				!reflect.DeepEqual(shortages, refusal.Error.Details.Shortages) ||
				!reflect.DeepEqual(issues, refusal.Error.Details.Issues) {
				t.Fatalf("attempt snapshot mismatch: %s %s %s", itemsData, shortagesData, issuesData)
			}
			if !reflect.DeepEqual(before, orderWriteSnapshot(t, f)) {
				t.Fatal("refusal changed stock, orders, audit or outbox")
			}
			if testutil.ToFloat64(telemetry.Default.Refusals.WithLabelValues(scenario.code)) != metricBefore+1 {
				t.Fatal("new committed refusal not counted")
			}
			repeat := f.createRequest(body, key)
			if testutil.ToFloat64(telemetry.Default.Refusals.WithLabelValues(scenario.code)) != metricBefore+1 {
				t.Fatal("replay counted twice")
			}
			if repeat.Code != response.Code || !bytes.Equal(repeat.Body.Bytes(), response.Body.Bytes()) {
				t.Fatalf("refusal replay differs: %d %s", repeat.Code, repeat.Body.String())
			}
			if f.count(t, "order_attempts") != 1 || f.count(t, "idempotency_keys") != 1 {
				t.Fatal("refusal replay created more facts")
			}
		})
	}
}

func TestOrderRefusalIntegrationUnknownDishDoesNotRecordAttemptOrKey(t *testing.T) {
	f := newCreationFixture(t)
	before := f.snapshot(t)
	response := f.createRequest(orderBody(t, dto.OrderRequestItem{DishID: uuid.NewString(), Portions: 1}), uuid.New())
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown dish: %d %s", response.Code, response.Body.String())
	}
	if f.snapshot(t) != before {
		t.Fatal("field validation recorded a refusal or idempotency key")
	}
}

func TestFailedRefusalCommitDoesNotIncrementMetric(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	f.exec(
		t,
		`CREATE FUNCTION reject_attempt_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test secret'; END $$;
 CREATE CONSTRAINT TRIGGER reject_attempt_commit AFTER INSERT ON order_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_attempt_commit()`,
	)
	before := testutil.ToFloat64(telemetry.Default.Refusals.WithLabelValues("INSUFFICIENT_STOCK"))
	response := f.createRequest(orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1}), uuid.New())
	if response.Code != 500 ||
		testutil.ToFloat64(telemetry.Default.Refusals.WithLabelValues("INSUFFICIENT_STOCK")) != before ||
		f.count(t, "order_attempts") != 0 {
		t.Fatal("rolled back refusal counted", response.Code)
	}
}
