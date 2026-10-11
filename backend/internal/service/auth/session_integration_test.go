package auth_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

type fixture struct {
	ctx            context.Context
	pool           *pgxpool.Pool
	repo           *authrepository.Repository
	service        *authservice.Service
	user           uuid.UUID
	password, hash string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("FLAGMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FLAGMAN_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	if (cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "::1" && cfg.ConnConfig.Host != "localhost") || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("auth tests require a dedicated local test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	name := "auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg = cfg.Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("missing migrations")
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{ctx: ctx, pool: pool, user: uuid.New(), password: "correct test password"}
	f.hash, err = authservice.Hash(f.password)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Test Manager','manager@example.test',$2,'manager')`, f.user, f.hash)
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name) VALUES('Test Restaurant')`)
	f.repo = authrepository.NewRepository(pool)
	f.service, err = authservice.NewService(f.repo, "https://flagman.example")
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestSessionIssuanceHashLifetimeAndRevocation(t *testing.T) {
	f := newFixture(t)
	session, token, err := f.service.Authenticate(f.ctx, " MANAGER@EXAMPLE.TEST ", f.password)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || len(session.CSRFToken) != 43 || token == session.CSRFToken || session.Actor.ID != f.user || session.User.LastLoginAt == nil || session.ExpiresAt.Sub(*session.User.LastLoginAt) != 12*time.Hour {
		t.Fatal("invalid issued session")
	}
	var stored, csrf string
	if err := f.pool.QueryRow(f.ctx, `SELECT token_hash,csrf_token FROM sessions WHERE user_id=$1`, f.user).Scan(&stored, &csrf); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != authservice.SessionTokenHash(token) || csrf != session.CSRFToken {
		t.Fatal("unsafe session storage")
	}
	current, err := f.service.CurrentSession(f.ctx, token)
	if err != nil || current.User.ID != f.user || current.CSRFToken != session.CSRFToken || current.Settings.RestaurantName != "Test Restaurant" {
		t.Fatal("session lookup failed")
	}
	_, second, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil || second == token {
		t.Fatal("session tokens are not independent")
	}
	if err := f.service.Revoke(f.ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CurrentSession(f.ctx, token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("revoked session accepted")
	}
	if _, err := f.service.CurrentSession(f.ctx, second); err != nil {
		t.Fatal("logout revoked another session")
	}
	f.exec(t, `UPDATE sessions SET created_at=clock_timestamp()-interval '13 hours',expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, authservice.SessionTokenHash(second))
	if _, err := f.service.CurrentSession(f.ctx, second); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("expired session accepted")
	}
}

func TestRejectedCredentialsAndAtomicIssuance(t *testing.T) {
	f := newFixture(t)
	for _, input := range [][2]string{{"manager@example.test", "wrong password"}, {"unknown@example.test", f.password}} {
		if _, _, err := f.service.Authenticate(f.ctx, input[0], input[1]); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatal("invalid credentials accepted")
		}
	}
	f.exec(t, `UPDATE users SET is_active=false WHERE id=$1`, f.user)
	if _, _, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("disabled user accepted")
	}
	f.exec(t, `UPDATE users SET is_active=true WHERE id=$1`, f.user)
	if _, err := f.repo.CreateSession(f.ctx, f.user, "stale-password", authservice.SessionTokenHash("token"), "csrf"); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("stale password accepted")
	}
	f.exec(t, `ALTER TABLE sessions ADD CONSTRAINT reject_test_session CHECK(csrf_token <> 'rejected')`)
	if _, err := f.repo.CreateSession(f.ctx, f.user, f.hash, authservice.SessionTokenHash("token"), "rejected"); err == nil {
		t.Fatal("expected session insert failure")
	}
	var count int
	var last *time.Time
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM sessions),last_login_at FROM users WHERE id=$1`, f.user).Scan(&count, &last); err != nil {
		t.Fatal(err)
	}
	if count != 0 || last != nil {
		t.Fatal("failed login partially persisted")
	}
}
