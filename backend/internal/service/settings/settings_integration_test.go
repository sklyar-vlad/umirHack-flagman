package settings_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

func ptr[T any](v T) *T { return &v }

func TestSettingsPartialFieldsBoundariesAndRevision(t *testing.T) {
	f := newFixture(t)
	before, err := f.settings.Get(f.actor())
	if err != nil || before.Revision != 1 || before.Timezone != "Europe/Moscow" || before.AutoWriteOffExpired {
		t.Fatalf("initial settings: %+v %v", before, err)
	}
	for _, in := range []dto.UpdateSettingsRequest{
		{RestaurantName: ptr("  Бистро №12  "), ExpectedRevision: 1},
		{Timezone: ptr("America/New_York"), ExpectedRevision: 2},
		{ExpiringThresholdHours: ptr(1), ExpectedRevision: 3},
		{ExpiringThresholdHours: ptr(720), ExpectedRevision: 4},
		{RestaurantName: ptr(strings.Repeat("Я", 200)), AutoWriteOffExpired: ptr(false), ExpectedRevision: 5},
	} {
		got, err := f.settings.Update(f.actor(), in)
		if err != nil || got.Revision != in.ExpectedRevision+1 {
			t.Fatalf("partial update: %+v %v", got, err)
		}
	}
	got, err := f.settings.Get(f.actor())
	if err != nil || got.Timezone != "America/New_York" || got.ExpiringThresholdHours != 720 ||
		got.RestaurantName != strings.Repeat("Я", 200) {
		t.Fatalf("settings: %+v %v", got, err)
	}
	for _, in := range []dto.UpdateSettingsRequest{
		{ExpectedRevision: 6},
		{RestaurantName: ptr(" "), ExpectedRevision: 6},
		{RestaurantName: ptr(strings.Repeat("Я", 201)), ExpectedRevision: 6},
		{RestaurantName: ptr("a\x00b"), ExpectedRevision: 6},
		{RestaurantName: ptr("a\nb"), ExpectedRevision: 6},
		{RestaurantName: ptr(string([]byte{255})), ExpectedRevision: 6},
		{Timezone: ptr("Local"), ExpectedRevision: 6},
		{Timezone: ptr(""), ExpectedRevision: 6},
		{Timezone: ptr("Mars/Unknown"), ExpectedRevision: 6},
		{Timezone: ptr("../UTC"), ExpectedRevision: 6},
		{Timezone: ptr("/etc/localtime"), ExpectedRevision: 6},
		{ExpiringThresholdHours: ptr(0), ExpectedRevision: 6},
		{ExpiringThresholdHours: ptr(721), ExpectedRevision: 6},
		{ExpiringThresholdHours: ptr(24), ExpectedRevision: 0},
		{ExpiringThresholdHours: ptr(24), ExpectedRevision: 2147483647},
	} {
		if _, err := f.settings.Update(f.actor(), in); err == nil {
			t.Fatalf("accepted invalid input: %+v", in)
		}
	}
	_, err = f.settings.Update(f.actor(), dto.UpdateSettingsRequest{Timezone: ptr("UTC"), ExpectedRevision: 1})
	var conflict *apperrors.RevisionConflict
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 6 {
		t.Fatalf("stale revision: %v", err)
	}
	final, _ := f.settings.Get(f.actor())
	if final != got {
		t.Fatal("invalid updates changed settings")
	}
	var count, audits int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM restaurant_settings),(SELECT count(*) FROM audit_log)`).
		Scan(&count, &audits); err != nil ||
		count != 1 ||
		audits != 5 {
		t.Fatalf("singleton/audit: %d %d %v", count, audits, err)
	}
}

func TestSettingsRolesStrictHTTPAndCSRF(t *testing.T) {
	f := newFixture(t)
	router := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "GET", "", token, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":1`) {
			t.Fatalf("%s read: %d %s", role, w.Code, w.Body)
		}
		if _, err := f.settings.Get(requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: role})); err != nil {
			t.Fatal(err)
		}
		if role != "admin" {
			w = request(
				router,
				"PATCH",
				`{"restaurant_name":"Changed","expected_revision":1}`,
				token,
				session.CSRFToken,
				"https://flagman.example",
			)
			if w.Code != 403 {
				t.Fatalf("%s write: %d", role, w.Code)
			}
			if _, err := f.settings.Update(
				requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: role}),
				dto.UpdateSettingsRequest{RestaurantName: ptr("Changed"), ExpectedRevision: 1},
			); !errors.Is(
				err,
				apperrors.ErrForbidden,
			) {
				t.Fatalf("service role: %v", err)
			}
			continue
		}
		for _, input := range [][2]string{{"", ""}, {"https://flagman.example", "wrong"}, {"https://evil.example", session.CSRFToken}} {
			if w := request(
				router,
				"PATCH",
				`{"restaurant_name":"Changed","expected_revision":1}`,
				token,
				input[1],
				input[0],
			); w.Code != 403 {
				t.Fatal("missing Origin/CSRF accepted")
			}
		}
		for _, body := range []string{`null`, `[]`, `{`, `{"restaurant_name":null,"expected_revision":1}`, `{"timezone":"UTC","expected_revision":1,"expected_revision":1}`, `{"Timezone":"UTC","expected_revision":1}`, `{"extra":true}`, `{"expiring_threshold_hours":1.5,"expected_revision":1}`, `{"auto_writeoff_expired":"false","expected_revision":1}`, `{"expected_revision":1}`, `{"restaurant_name":"x","expected_revision":1} {}`} {
			w := request(router, "PATCH", body, token, session.CSRFToken, "https://flagman.example")
			if w.Code != 400 && w.Code != 422 {
				t.Fatalf("invalid body: %s: %d %s", body, w.Code, w.Body)
			}
		}
	}
	if w := request(router, "GET", "", "", "", ""); w.Code != 401 {
		t.Fatal("public settings")
	}
	if _, err := f.settings.Get(f.ctx); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("service public settings")
	}
}

func TestConcurrentSettingsSavesHaveOneWinner(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := f.settings.Update(
				f.actor(),
				dto.UpdateSettingsRequest{RestaurantName: ptr(fmt.Sprintf("Name %d", i)), ExpectedRevision: 1},
			)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		var conflict *apperrors.RevisionConflict
		switch {
		case err == nil:
			successes++
		case errors.As(err, &conflict) && conflict.CurrentRevision == 2:
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success/conflict: %d %d", successes, conflicts)
	}
	var audits int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_log WHERE entity_type='settings'`).
		Scan(&audits); err != nil ||
		audits != 1 {
		t.Fatalf("audit count %d %v", audits, err)
	}
}
