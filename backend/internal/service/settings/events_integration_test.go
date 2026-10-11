package settings_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	settingshandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/settings"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSettingsTypedBeforeAfterAndActorAreAtomic(t *testing.T) {
	f := newFixture(t)
	before, _ := f.settings.Get(f.actor())
	ctx := requestctx.WithID(f.actor(), "settings-save")
	after, err := f.settings.Update(
		ctx,
		dto.UpdateSettingsRequest{
			RestaurantName:         ptr("After"),
			Timezone:               ptr("UTC"),
			ExpiringThresholdHours: ptr(48),
			ExpectedRevision:       1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var actor uuid.UUID
	var entity *uuid.UUID
	var action, typ, rid string
	var old, next []byte
	if err := f.pool.QueryRow(ctx, `SELECT actor_id,entity_id,action,entity_type,request_id,before_data,after_data FROM audit_log`).
		Scan(&actor, &entity, &action, &typ, &rid, &old, &next); err != nil {
		t.Fatal(err)
	}
	if actor != f.user || entity != nil || action != "update" || typ != "settings" || rid != "settings-save" {
		t.Fatal("audit metadata")
	}
	var beforeDTO, afterDTO dto.Settings
	var fields map[string]any
	if err := json.Unmarshal(old, &beforeDTO); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(next, &afterDTO); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(next, &fields); err != nil {
		t.Fatal(err)
	}
	if beforeDTO != before || afterDTO != after || len(fields) != 5 {
		t.Fatal("typed snapshots", string(next))
	}
}

func TestSettingsEventFailureRollsBackAndLogsOnlySQLState(t *testing.T) {
	for _, table := range []string{"audit_log", "outbox_events"} {
		for _, deferred := range []bool{false, true} {
			t.Run(table+map[bool]string{true: " deferred", false: " immediate"}[deferred], func(t *testing.T) {
				f := newFixture(t)
				f.exec(
					t,
					`CREATE FUNCTION reject_settings_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test storage secret'; END $$`,
				)
				if deferred {
					f.exec(
						t,
						`CREATE CONSTRAINT TRIGGER reject_settings_event AFTER INSERT ON `+table+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_settings_event()`,
					)
				} else {
					f.exec(
						t,
						`CREATE TRIGGER reject_settings_event BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION reject_settings_event()`,
					)
				}
				core, logs := observer.New(zap.DebugLevel)
				r := gin.New()
				r.Use(basehandler.RequestID(), basehandler.RequestLogger(zap.New(core)))
				access := authhandler.NewHandler(f.service)
				api := r.Group("/api/v1", access.RequireRouteAccess)
				basehandler.RegisterSettingsRoutes(api, settingshandler.NewHandler(f.settings))
				session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
				if err != nil {
					t.Fatal(err)
				}
				w := request(
					r,
					"PATCH",
					`{"restaurant_name":"Changed","timezone":"UTC","expiring_threshold_hours":48,"expected_revision":1}`,
					token,
					session.CSRFToken,
					"https://flagman.example",
				)
				if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
					t.Fatal(w.Code, w.Body)
				}
				got, err := f.settings.Get(f.actor())
				if err != nil || got.Revision != 1 || got.RestaurantName != "Test Restaurant" ||
					got.Timezone != "Europe/Moscow" ||
					got.ExpiringThresholdHours != 24 {
					t.Fatal("mutation persisted", got, err)
				}
				var audits, events int
				if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events)`).
					Scan(&audits, &events); err != nil ||
					audits != 0 ||
					events != 0 {
					t.Fatal("events persisted", audits, events, err)
				}
				contexts := make([]map[string]any, 0, logs.Len())
				for _, entry := range logs.All() {
					contexts = append(contexts, entry.ContextMap())
				}
				encoded, _ := json.Marshal(contexts)
				if strings.Contains(string(encoded), "private test storage secret") ||
					strings.Contains(string(encoded), token) ||
					strings.Contains(string(encoded), f.hash) ||
					!strings.Contains(string(encoded), "SQLSTATE P0001") {
					t.Fatal("unsafe or missing error log")
				}
			})
		}
	}
}

func waitLock(t *testing.T, f *fixture, query string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE $2`, f.schema, query).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected lock wait")
}

func TestSettingsActorRecheckedAfterWait(t *testing.T) {
	for _, change := range []string{"role='manager'", "is_active=false"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			if _, err := tx.Exec(f.ctx, `UPDATE users SET `+change+` WHERE id=$1`, f.user); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, err := f.settings.Update(
					f.actor(),
					dto.UpdateSettingsRequest{RestaurantName: ptr("Denied"), ExpectedRevision: 1},
				)
				result <- err
			}()
			waitLock(t, f, "%FROM users%FOR SHARE%")
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatal("stale actor wrote", err)
			}
			var revision, audits int
			if err := f.pool.QueryRow(f.ctx, `SELECT revision,(SELECT count(*) FROM audit_log) FROM restaurant_settings`).
				Scan(&revision, &audits); err != nil ||
				revision != 1 ||
				audits != 0 {
				t.Fatal("forbidden mutation wrote", revision, audits, err)
			}
		})
	}
}

func TestSettingsRevisionRecheckedAfterWait(t *testing.T) {
	f := newFixture(t)
	before, _ := f.settings.Get(f.actor())
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err := tx.Exec(
		f.ctx,
		`UPDATE restaurant_settings SET restaurant_name='Other writer',revision=revision+1 WHERE id=1`,
	); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.settings.Update(
			f.actor(),
			dto.UpdateSettingsRequest{RestaurantName: ptr("Stale"), ExpectedRevision: 1},
		)
		result <- err
	}()
	waitLock(t, f, "%FROM restaurant_settings%FOR UPDATE%")
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	var conflict *apperrors.RevisionConflict
	if err := <-result; !errors.As(err, &conflict) || conflict.CurrentRevision != 2 {
		t.Fatal("stale revision wrote", err)
	}
	got, err := f.settings.Get(f.actor())
	before.RestaurantName = "Other writer"
	before.Revision = 2
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatal(got, err)
	}
}
