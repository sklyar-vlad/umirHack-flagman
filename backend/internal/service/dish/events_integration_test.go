package dish_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	dishhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/dish"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDishTypedAuditAndAvailabilityOutbox(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "First", "g", true)
	p2 := f.product(t, "Second", "ml", true)
	ctx := requestctx.WithID(f.actor(), "dish-save")
	d, err := f.dishes.Create(
		ctx,
		dto.CreateDishRequest{Name: "Before", Description: ptr("before description"), Recipe: recipe(p, "1")},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.dishes.Update(
		ctx,
		d.ID,
		dto.UpdateDishRequest{
			ExpectedRevision: 1,
			Name:             ptr("After"),
			Description:      dto.Nullable[string]{Set: true},
			Recipe:           ptr(recipe(p2, "0.001")),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.dishes.Update(ctx, d.ID, dto.UpdateDishRequest{ExpectedRevision: 2, IsActive: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	f.dishes.Update(ctx, d.ID, dto.UpdateDishRequest{ExpectedRevision: 2, Name: ptr("Stale")})
	f.dishes.Preview(ctx, dto.PreviewDishRequest{Recipe: recipe(p2, "1")})
	rows, err := f.pool.Query(
		ctx,
		`SELECT actor_id,entity_id,action,entity_type,request_id,before_data,after_data FROM audit_log ORDER BY created_at,id`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []model.DishAuditSnapshot
	for rows.Next() {
		var actor, entity uuid.UUID
		var action, typ, rid string
		var before, after []byte
		if err := rows.Scan(&actor, &entity, &action, &typ, &rid, &before, &after); err != nil {
			t.Fatal(err)
		}
		i := len(snapshots)
		want := "update"
		if i == 0 {
			want = "create"
		}
		if actor != f.user || entity != d.ID || action != want || typ != "dish" || rid != "dish-save" {
			t.Fatal("audit metadata")
		}
		var next model.DishAuditSnapshot
		var fields map[string]any
		if err := json.Unmarshal(after, &next); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(after, &fields)
		if len(fields) != 7 || next.Revision != i+1 || len(next.Recipe) != 1 {
			t.Fatal("snapshot allowlist", string(after))
		}
		if i == 0 {
			if before != nil || next.Recipe[0].ProductID != p || next.Recipe[0].QuantityPerPortion != "1.000" ||
				*next.RecipeVersion != 1 {
				t.Fatal("create snapshot")
			}
		} else {
			var old model.DishAuditSnapshot
			if err := json.Unmarshal(before, &old); err != nil || !reflect.DeepEqual(old, snapshots[i-1]) {
				t.Fatal("before snapshot", err)
			}
			if next.Name != "After" || next.Description != nil || *next.RecipeVersion != 2 ||
				next.Recipe[0].ProductID != p2 ||
				next.Recipe[0].QuantityPerPortion != "0.001" {
				t.Fatal("after snapshot")
			}
			if i == 1 && (*next.RecipeVersionID == *old.RecipeVersionID || !next.IsActive) {
				t.Fatal("recipe version ID not changed")
			}
			if i == 2 && (*next.RecipeVersionID != *old.RecipeVersionID || next.IsActive) {
				t.Fatal("metadata edit version or activity")
			}
		}
		snapshots = append(snapshots, next)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(snapshots) != 3 {
		t.Fatal("failed writes or preview audited")
	}
	rows, err = f.pool.Query(ctx, `SELECT event_type,actor_id,payload FROM outbox_events ORDER BY cursor`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var typ string
		var actor uuid.UUID
		var raw []byte
		if err := rows.Scan(&typ, &actor, &raw); err != nil {
			t.Fatal(err)
		}
		var payload model.OutboxPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if typ != "dishes.changed" || actor != f.user || payload.RequestID != "dish-save" ||
			!slices.Equal(payload.DishIDs, []uuid.UUID{d.ID}) ||
			payload.OccurredAt.IsZero() ||
			!slices.Equal(
				payload.Topics,
				[]model.EventTopic{
					model.TopicDishes,
					model.TopicStopList,
					model.TopicHotOffers,
					model.TopicDashboard,
					model.TopicProcurementRecommendations,
				},
			) {
			t.Fatal("outbox metadata/topics", string(raw))
		}
		want := []uuid.UUID{p}
		if i == 1 {
			want = []uuid.UUID{p, p2}
			slices.SortFunc(want, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		}
		if i == 2 {
			want = []uuid.UUID{p2}
		}
		if !slices.Equal(payload.ProductIDs, want) {
			t.Fatal("affected product IDs", string(raw))
		}
		i++
	}
	if rows.Err() != nil || i != 3 {
		t.Fatal("outbox count", i, rows.Err())
	}
}

func TestDishAuditAndOutboxFailureRollsBackIncludingDeferredCommit(t *testing.T) {
	for _, table := range []string{"audit_log", "outbox_events"} {
		for _, deferred := range []bool{false, true} {
			for _, patch := range []bool{false, true} {
				t.Run(
					table+map[bool]string{false: " immediate", true: " deferred"}[deferred]+map[bool]string{false: " create", true: " patch"}[patch],
					func(t *testing.T) {
						f := newFixture(t)
						p := f.product(t, "Ingredient", "g", true)
						d := f.createDish(t, p)
						f.exec(
							t,
							`CREATE FUNCTION reject_dish_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private dish storage detail'; END $$`,
						)
						if deferred {
							f.exec(
								t,
								`CREATE CONSTRAINT TRIGGER reject_dish_event AFTER INSERT ON `+table+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_dish_event()`,
							)
						} else {
							f.exec(
								t,
								`CREATE TRIGGER reject_dish_event BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION reject_dish_event()`,
							)
						}
						core, logs := observer.New(zap.DebugLevel)
						router := gin.New()
						router.Use(basehandler.RequestID(), basehandler.RequestLogger(zap.New(core)))
						access := authhandler.NewHandler(f.service)
						api := router.Group("/api/v1", access.RequireRouteAccess)
						basehandler.RegisterDishCreateRoutes(api, dishhandler.NewHandler(f.dishes))
						basehandler.RegisterDishUpdateRoutes(api, dishhandler.NewHandler(f.dishes))
						session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
						if err != nil {
							t.Fatal(err)
						}
						method, path, body := "POST", "/dishes", `{"name":"Failed","recipe":[{"product_id":"`+p.String()+`","quantity_per_portion":"2"}]}`
						if patch {
							method = "PATCH"
							path += "/" + d.ID.String()
							body = `{"expected_revision":1,"name":"Failed","description":null,"is_active":false,"recipe":[{"product_id":"` + p.String() + `","quantity_per_portion":"2"}]}`
						}
						w := request(router, method, path, body, token, session.CSRFToken, "https://flagman.example")
						if w.Code != 500 || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
							t.Fatalf("failure: %d %s", w.Code, w.Body.String())
						}
						if f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 ||
							f.count(t, "recipe_items") != 1 ||
							f.count(t, "audit_log") != 1 ||
							f.count(t, "outbox_events") != 1 {
							t.Fatal("partial event/mutation survived")
						}
						var name string
						var revision int
						var active bool
						var description *string
						if err := f.pool.QueryRow(f.ctx, `SELECT name,revision,is_active,description FROM dishes WHERE id=$1`, d.ID).
							Scan(&name, &revision, &active, &description); err != nil {
							t.Fatal(err)
						}
						if name != "Original" || revision != 1 || !active || description == nil {
							t.Fatal("metadata did not roll back")
						}
						for _, entry := range logs.All() {
							raw, _ := json.Marshal(entry.ContextMap())
							if strings.Contains(string(raw), "private dish storage detail") ||
								strings.Contains(string(raw), token) ||
								strings.Contains(string(raw), f.hash) {
								t.Fatal("database detail or secret logged")
							}
						}
						if logs.Len() == 0 || strings.Contains(w.Body.String(), "private dish storage detail") {
							t.Fatal("missing safe log or detail response")
						}
					},
				)
			}
		}
	}
}
