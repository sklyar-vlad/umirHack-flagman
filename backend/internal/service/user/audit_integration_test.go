package user_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	userhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/user"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSafeUserAuditSnapshotsAndFailedMutationAbsence(t *testing.T) {
	f := newFixture(t)
	ctx := requestctx.WithID(f.actor(), "user-audit-request")
	u, err := f.users.Create(ctx, createInput())
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.service.Authenticate(ctx, u.Email, createInput().Password)
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Role: ptr("storekeeper"), ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	next, err = f.users.Password(
		ctx,
		u.ID,
		dto.UserPasswordRequest{Password: "reset password value", ExpectedRevision: next.Revision},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Refused/stale/invalid/duplicate requests leave no successful audit entries.
	f.users.Update(ctx, f.user, dto.UpdateUserRequest{IsActive: ptr(false), ExpectedRevision: 1})
	f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Name: ptr("stale"), ExpectedRevision: 1})
	f.users.Create(ctx, createInput())
	f.users.Password(ctx, u.ID, dto.UserPasswordRequest{Password: "short", ExpectedRevision: 3})
	rows, err := f.pool.Query(
		ctx,
		`SELECT actor_id,action,entity_type,entity_id,request_id,before_data,after_data FROM audit_log ORDER BY created_at,id`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actions := []string{"create", "update", "password_change"}
	i := 0
	for rows.Next() {
		var actor, entity uuid.UUID
		var action, typ, id string
		var before, after []byte
		if err := rows.Scan(&actor, &action, &typ, &entity, &id, &before, &after); err != nil {
			t.Fatal(err)
		}
		if i >= len(actions) || actions[i] != action || actor != f.user || entity != u.ID || typ != "user" ||
			id != "user-audit-request" {
			t.Fatal("incorrect audit metadata", action, typ, id)
		}
		if i == 0 && before != nil {
			t.Fatal("create audit before")
		}
		if i > 0 && before == nil {
			t.Fatal("update audit before missing")
		}
		var snapshot map[string]any
		if json.Unmarshal(after, &snapshot) != nil || len(snapshot) != 5 || snapshot["revision"] != float64(i+1) {
			t.Fatal("invalid safe snapshot", string(after))
		}
		for _, secret := range []string{"password_hash", createInput().Password, "reset password value", token, "csrf_token", "token_hash", "argon2id"} {
			if strings.Contains(string(before)+string(after), secret) {
				t.Fatal("unsafe audit snapshot")
			}
		}
		if i == 1 && snapshot["role"] != "storekeeper" {
			t.Fatal("role missing from audit")
		}
		i++
	}
	if rows.Err() != nil || i != 3 {
		t.Fatal("incorrect audit count", i, rows.Err())
	}
}

func TestUserAuditFailureRollsBackAllMutationsAndLogsNoSecrets(t *testing.T) {
	for _, kind := range []string{"create", "update", "password", "deferred"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			ctx := f.actor()
			u, err := f.users.Create(ctx, createInput())
			if err != nil {
				t.Fatal(err)
			}
			session, token, err := f.service.Authenticate(ctx, "manager@example.test", f.password)
			if err != nil {
				t.Fatal(err)
			}
			_, userToken, err := f.service.Authenticate(ctx, u.Email, createInput().Password)
			if err != nil {
				t.Fatal(err)
			}
			f.exec(
				t,
				`CREATE FUNCTION reject_user_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit test secret initial password 123 argon2id'; END $$`,
			)
			if kind == "deferred" {
				f.exec(
					t,
					`CREATE CONSTRAINT TRIGGER reject_audit AFTER INSERT ON audit_log DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_user_audit()`,
				)
			} else {
				f.exec(
					t,
					`CREATE TRIGGER reject_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_user_audit()`,
				)
			}
			core, logs := observer.New(zap.DebugLevel)
			r := gin.New()
			r.Use(basehandler.RequestID(), basehandler.RequestLogger(zap.New(core)))
			access := authhandler.NewHandler(f.service)
			r.Use(access.CORS())
			api := r.Group("/api/v1", access.RequireRouteAccess)
			basehandler.RegisterUserWriteRoutes(api, userhandler.NewHandler(f.users))
			method, path, body := "POST", "/users", `{"name":"Failed","email":"failed@example.test","role":"manager","password":"initial password 123"}`
			if kind == "update" {
				method = "PATCH"
				path = "/users/" + u.ID.String()
				body = `{"role":"storekeeper","expected_revision":1}`
			}
			if kind == "password" || kind == "deferred" {
				path = "/users/" + u.ID.String() + "/password"
				body = `{"password":"initial password 123 changed","expected_revision":1}`
			}
			w := request(r, method, path, body, token, session.CSRFToken, "https://flagman.example")
			if w.Code != 500 || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
				t.Fatal("audit failure HTTP", w.Code, w.Body)
			}
			var count, rev, unrevoked int
			var role string
			f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),revision,role::text,(SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL) FROM users WHERE id=$1`, u.ID).
				Scan(&count, &rev, &role, &unrevoked)
			if count != 2 || rev != 1 || role != "manager" || unrevoked != 1 {
				t.Fatal("audit failure partial writes", count, rev, role, unrevoked)
			}
			if _, err := f.service.CurrentSession(ctx, userToken); err != nil {
				t.Fatal("audit failure revoked session")
			}
			if _, _, err := f.service.Authenticate(ctx, u.Email, createInput().Password); err != nil {
				t.Fatal("audit failure changed password")
			}
			var audits int
			f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&audits)
			if audits != 1 {
				t.Fatal("failed audit persisted")
			}
			for _, entry := range logs.All() {
				data, _ := json.Marshal(entry.ContextMap())
				content := entry.Message + string(data)
				for _, secret := range []string{"audit test secret", "initial password 123", "argon2id", token, session.CSRFToken} {
					if strings.Contains(content, secret) {
						t.Fatal("secret logged")
					}
				}
			}
		})
	}
}

func TestAdminSelfPasswordResetRevokesCurrentSession(t *testing.T) {
	f := newFixture(t)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	w := request(
		f.router(),
		"POST",
		"/users/"+f.user.String()+"/password",
		`{"password":"self reset password","expected_revision":1}`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 200 {
		t.Fatal("self reset", w.Code, w.Body)
	}
	if _, err := f.service.CurrentSession(f.ctx, token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("self-reset session survived")
	}
	if _, _, err := f.service.Authenticate(f.ctx, "manager@example.test", "self reset password"); err != nil {
		t.Fatal("reset admin login", err)
	}
}
