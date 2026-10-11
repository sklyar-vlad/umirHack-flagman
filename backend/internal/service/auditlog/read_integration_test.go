package auditlog_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	auditrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auditlog"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	auditservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auditlog"
	"go.uber.org/zap"
)

func ptr[T any](v T) *T { return &v }
func list(t *testing.T, f *fixture, query string) dto.ListResponse[dto.AuditLogEntry] {
	t.Helper()
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	w := request(f.router(), "GET", "/audit-log"+query, "", token, session.CSRFToken, "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body)
	}
	var result dto.ListResponse[dto.AuditLogEntry]
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func seedAudit(t *testing.T, f *fixture) (uuid.UUID, time.Time) {
	t.Helper()
	base := repository.NewRepository(f.pool, zap.NewNop())
	entity := uuid.New()
	ctx := requestctx.WithID(f.actor(), "human-request")
	err := base.WithinTx(ctx, nil, func(tx repository.DBTX) error {
		if err := base.AppendAudit(ctx, tx, &model.AuditEntry{ActorID: &f.user, Action: model.AuditCreate, EntityType: model.AuditProductCategory, EntityID: &entity, After: &model.CategoryAuditSnapshot{Name: "First", Revision: 1}}); err != nil {
			return err
		}
		if err := base.AppendAudit(ctx, tx, &model.AuditEntry{ActorID: &f.user, Action: model.AuditUpdate, EntityType: model.AuditProductCategory, EntityID: &entity, Before: &model.CategoryAuditSnapshot{Name: "First", Revision: 1}, After: &model.CategoryAuditSnapshot{Name: "Next", Revision: 2}}); err != nil {
			return err
		}
		// A background operation has no user or request id and remains visible.
		return base.AppendAudit(f.ctx, tx, &model.AuditEntry{Action: model.AuditExpiration, EntityType: model.AuditStockMovement, EntityID: ptr(uuid.New()), After: &model.StockMovementAuditSnapshot{BatchID: uuid.New(), Type: "expiration", DeltaQuantity: "-1.000", RemainingQuantityAfter: "0.000", ProductNameSnapshot: "Old", UnitSnapshot: "g"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	f.exec(t, `UPDATE audit_log SET created_at=$1`, at)
	f.exec(t, `UPDATE users SET name=$2 WHERE id=$1`, f.user, `Автор 100%_\`)
	return entity, at
}
func TestAuditFiltersLiteralSearchAndStablePagination(t *testing.T) {
	f := newFixture(t)
	entity, at := seedAudit(t, f)
	all := list(t, f, "?page_size=2")
	if all.Total != 3 || len(all.Items) != 2 || all.Page != 1 || all.PageSize != 2 {
		t.Fatal(all)
	}
	page2 := list(t, f, "?page=2&page_size=2")
	if page2.Total != 3 || len(page2.Items) != 1 {
		t.Fatal(page2)
	}
	ids := map[uuid.UUID]bool{}
	for _, item := range append(all.Items, page2.Items...) {
		if ids[item.ID] {
			t.Fatal("duplicate page item")
		}
		ids[item.ID] = true
	}
	combined := append(all.Items, page2.Items...)
	for i := 1; i < len(combined); i++ {
		if strings.Compare(combined[i-1].ID.String(), combined[i].ID.String()) <= 0 {
			t.Fatal("unstable tie order")
		}
	}
	cases := []struct {
		query string
		total int64
	}{
		{"actor_id=" + f.user.String(), 2}, {"actor_id=system", 1}, {"actor_id=" + uuid.NewString(), 0},
		{"action=create", 1}, {"action=update", 1}, {"action=expiration", 1}, {"action=password_change", 0},
		{"entity_type=product_category", 2}, {"entity_type=stock_movement", 1}, {"entity_id=" + entity.String(), 2},
		{"entity_type=product_category&action=update&actor_id=" + f.user.String() + "&entity_id=" + entity.String(), 1},
		{"search=" + url.QueryEscape(`100%_\`), 2}, {"search=" + url.QueryEscape(`100XYZ`), 0},
		{"search=SYSTEM", 1}, {"search=" + url.QueryEscape("система"), 1}, {"search=human-request", 2},
		{"search=" + entity.String(), 2}, {"search=%27%20OR%201%3D1--", 0},
		{"from=" + url.QueryEscape(at.Format(time.RFC3339)) + "&to=" + url.QueryEscape(at.Add(time.Second).Format(time.RFC3339)), 3},
		{"to=" + url.QueryEscape(at.Format(time.RFC3339)), 0}, {"from=" + url.QueryEscape(at.Add(time.Second).Format(time.RFC3339)), 0},
		{"from=" + url.QueryEscape(at.Format(time.RFC3339)) + "&to=" + url.QueryEscape(at.Format(time.RFC3339)), 0},
		{"page=100", 3},
	}
	for _, tc := range cases {
		got := list(t, f, "?"+tc.query)
		if got.Total != tc.total {
			t.Fatalf("%s: total %d want %d", tc.query, got.Total, tc.total)
		}
		if got.Items == nil {
			t.Fatal("null items")
		}
	}
	system := list(t, f, "?actor_id=system").Items[0]
	if system.Actor != nil || system.RequestID != nil || system.Action != "expiration" {
		t.Fatal("system actor", system)
	}
	user := list(t, f, "?action=update").Items[0]
	if user.Actor == nil || user.Actor.ID != f.user || user.Actor.Name != `Автор 100%_\` || user.RequestID == nil || *user.RequestID != "human-request" || !strings.Contains(string(user.Before), "First") || !strings.Contains(string(user.After), "Next") || user.EntityID == nil || *user.EntityID != entity {
		t.Fatal("human metadata", user)
	}
	// Every read leaves the source and events intact.
	var count, events int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM audit_log),(SELECT count(*) FROM outbox_events)`).Scan(&count, &events); err != nil || count != 3 || events != 0 {
		t.Fatal("read mutated", count, events, err)
	}
}
func TestAuditAdminPolicyAndMalformedFilters(t *testing.T) {
	f := newFixture(t)
	seedAudit(t, f)
	router := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "GET", "/audit-log", "", token, "", "")
		expected := 403
		if role == "admin" {
			expected = 200
		}
		if w.Code != expected || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d", role, w.Code)
		}
		_, _, err = f.audit.List(requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: role}), model.AuditLogFilter{Page: 1, PageSize: 20})
		if role == "admin" && err != nil || role != "admin" && !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("service policy", err)
		}
		if role != "admin" {
			continue
		}
		for _, q := range []string{"actor_id=", "actor_id=null", "entity_id=wrong", "action=delete", "action=", "entity_type=session", "entity_type=", "sort=created_at", "page=0", "page_size=101", "from=2026-10-11", "to=bad", "from=2026-10-12T00:00:00Z&to=2026-10-11T00:00:00Z", "actor_id=system&actor_id=system", "search=a&search=b", "extra=true", "search=%ZZ"} {
			w := request(router, "GET", "/audit-log?"+q, "", token, session.CSRFToken, "")
			if w.Code != 422 {
				t.Fatalf("%s: %d %s", q, w.Code, w.Body)
			}
		}
	}
	if w := request(router, "GET", "/audit-log", "", "", "", ""); w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("public audit", w.Code)
	}
	if _, _, err := f.audit.List(f.ctx, model.AuditLogFilter{Page: 1, PageSize: 20}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("public service audit", err)
	}
}

func TestAuditReadRemovesUnexpectedSecretsIncludingNestedFields(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `INSERT INTO audit_log(actor_id,action,entity_type,entity_id,created_at,after_data,before_data) VALUES($1,'password_change','user',$1,clock_timestamp(),$2,$2)`, f.user, `{"name":"Visible","email":"test@example.test","role":"manager","is_active":true,"revision":2,"password":"private-marker","password_hash":"private-marker","session_token":"private-marker","cookie":"private-marker"}`)
	f.exec(t, `INSERT INTO audit_log(action,entity_type,entity_id,created_at,after_data) VALUES('create','dish',$1,clock_timestamp(),$2)`, uuid.New(), `{"name":"Safe dish","revision":1,"recipe":[{"product_id":"00000000-0000-0000-0000-000000000001","quantity_per_portion":"1.000","password":"private-marker"}],"token":"private-marker"}`)
	got := list(t, f, "")
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || strings.Contains(string(raw), "private-marker") || strings.Contains(string(raw), `"password":`) || strings.Contains(string(raw), "cookie") || strings.Contains(string(raw), "session_token") || !strings.Contains(string(raw), "Visible") || !strings.Contains(string(raw), "quantity_per_portion") {
		t.Fatal("unsafe projection", string(raw))
	}
	// Unknown future/legacy entity types are safe too: no arbitrary snapshot JSON.
	f.exec(t, `INSERT INTO audit_log(action,entity_type,created_at,after_data) VALUES('legacy','session',clock_timestamp(),'{"token":"private-marker"}')`)
	got = list(t, f, "")
	raw, _ = json.Marshal(got)
	if strings.Contains(string(raw), "private-marker") {
		t.Fatal("unknown entity leaked")
	}
}

type snapshotTransactions struct {
	base interface {
		WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
	}
	beforePage func()
	t          *testing.T
}
type snapshotTx struct {
	pgx.Tx
	beforePage func()
}

func (tx snapshotTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "SELECT a.id") {
		tx.beforePage()
	}
	return tx.Tx.Query(ctx, sql, args...)
}
func (s snapshotTransactions) WithinTx(ctx context.Context, options *pgx.TxOptions, fn func(repository.DBTX) error) error {
	if options.IsoLevel != pgx.RepeatableRead || options.AccessMode != pgx.ReadOnly {
		s.t.Fatal("unsafe read transaction")
	}
	return s.base.WithinTx(ctx, options, func(q repository.DBTX) error {
		tx, err := repository.RequireTransaction(q)
		if err != nil {
			return err
		}
		return fn(snapshotTx{tx, s.beforePage})
	})
}
func TestAuditCountAndPageShareReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	seedAudit(t, f)
	base := repository.NewRepository(f.pool, zap.NewNop())
	hook := snapshotTransactions{base: base, t: t, beforePage: func() {
		f.exec(t, `INSERT INTO audit_log(action,entity_type,created_at,after_data) VALUES('update','settings',clock_timestamp(),'{"restaurant_name":"Concurrent","revision":2}')`)
	}}
	s := auditservice.NewService(auditrepository.NewRepository(), hook)
	got, total, err := s.List(f.actor(), model.AuditLogFilter{Page: 1, PageSize: 20})
	if err != nil || total != 3 || len(got) != 3 {
		t.Fatal("inconsistent count/page", total, len(got), err)
	}
	all := list(t, f, "")
	if all.Total != 4 {
		t.Fatal("concurrent insert did not happen")
	}
}
