package user_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	userrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/user"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	userservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/user"
	"go.uber.org/zap"
)

func TestLastAdminSingleAndConcurrentChanges(t *testing.T) {
	for _, disable := range []bool{false, true} {
		t.Run(map[bool]string{false: "demote", true: "disable"}[disable], func(t *testing.T) {
			f := newFixture(t)
			input := dto.UpdateUserRequest{ExpectedRevision: 1}
			if disable {
				input.IsActive = ptr(false)
			} else {
				input.Role = ptr("manager")
			}
			if _, err := f.users.Update(f.actor(), f.user, input); !errors.Is(err, apperrors.ErrLastAdmin) {
				t.Fatal("last admin changed", err)
			}
			// An inactive admin does not count as a surviving administrator.
			f.exec(
				t,
				`INSERT INTO users(name,email,password_hash,role,is_active) VALUES('Inactive','inactive@example.test',$1,'admin',false)`,
				f.hash,
			)
			if _, err := f.users.Update(f.actor(), f.user, input); !errors.Is(err, apperrors.ErrLastAdmin) {
				t.Fatal("inactive admin counted")
			}
			second, err := f.users.Create(
				f.actor(),
				dto.CreateUserRequest{
					Name:     "Second",
					Email:    "second@example.test",
					Role:     "admin",
					Password: f.password,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for _, id := range []uuid.UUID{f.user, second.ID} {
				wg.Add(1)
				go func(id uuid.UUID) {
					defer wg.Done()
					<-start
					ctx := requestctx.WithActor(f.ctx, model.Actor{ID: id, Role: "admin"})
					_, err := f.users.Update(ctx, id, input)
					results <- err
				}(id)
			}
			close(start)
			wg.Wait()
			close(results)
			success, protected := 0, 0
			for err := range results {
				switch {
				case err == nil:
					success++
				case errors.Is(err, apperrors.ErrLastAdmin):
					protected++
				default:
					t.Fatal(err)
				}
			}
			if success != 1 || protected != 1 {
				t.Fatal("concurrent last admin failure", success, protected)
			}
			var count int
			f.pool.QueryRow(f.ctx, `SELECT count(*) FROM users WHERE role='admin' AND is_active`).Scan(&count)
			if count != 1 {
				t.Fatal("lost every admin")
			}
		})
	}
}

func TestSessionRevocationAndProfileChanges(t *testing.T) {
	for _, kind := range []string{"role", "disable", "password"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			ctx := f.actor()
			u, err := f.users.Create(ctx, createInput())
			if err != nil {
				t.Fatal(err)
			}
			tokens := []string{}
			for i := 0; i < 2; i++ {
				_, token, err := f.service.Authenticate(f.ctx, u.Email, createInput().Password)
				if err != nil {
					t.Fatal(err)
				}
				tokens = append(tokens, token)
			}
			_, other, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
			if err != nil {
				t.Fatal(err)
			}
			next, err := f.users.Update(
				ctx,
				u.ID,
				dto.UpdateUserRequest{Name: ptr("Renamed"), Email: ptr("renamed@example.test"), ExpectedRevision: 1},
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range tokens {
				if session, err := f.service.CurrentSession(
					f.ctx,
					token,
				); err != nil ||
					session.User.Name != "Renamed" {
					t.Fatal("profile edit revoked session", err)
				}
			}
			switch kind {
			case "role":
				_, err = f.users.Update(
					ctx,
					u.ID,
					dto.UpdateUserRequest{Role: ptr("storekeeper"), ExpectedRevision: next.Revision},
				)
			case "disable":
				_, err = f.users.Update(
					ctx,
					u.ID,
					dto.UpdateUserRequest{IsActive: ptr(false), ExpectedRevision: next.Revision},
				)
			case "password":
				_, err = f.users.Password(
					ctx,
					u.ID,
					dto.UserPasswordRequest{Password: "new reset password", ExpectedRevision: next.Revision},
				)
			}
			if err != nil {
				t.Fatal(err)
			}
			var unrevoked int
			f.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, u.ID).
				Scan(&unrevoked)
			if unrevoked != 0 {
				t.Fatal("sessions not revoked")
			}
			for _, token := range tokens {
				if _, err := f.service.CurrentSession(f.ctx, token); !errors.Is(err, apperrors.ErrUnauthorized) {
					t.Fatal("revoked token accepted")
				}
			}
			if _, err := f.service.CurrentSession(f.ctx, other); err != nil {
				t.Fatal("another user's session revoked")
			}
			if kind == "disable" {
				_, err = f.users.Update(ctx, u.ID, dto.UpdateUserRequest{IsActive: ptr(true), ExpectedRevision: 3})
				if err != nil {
					t.Fatal(err)
				}
				for _, token := range tokens {
					if _, err := f.service.CurrentSession(f.ctx, token); !errors.Is(err, apperrors.ErrUnauthorized) {
						t.Fatal("reenable restored old session")
					}
				}
			}
		})
	}
}

func TestRevocationFailureRollsBackSecurityMutation(t *testing.T) {
	f := newFixture(t)
	ctx := f.actor()
	u, err := f.users.Create(ctx, createInput())
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.service.Authenticate(f.ctx, u.Email, createInput().Password)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `ALTER TABLE sessions ADD CONSTRAINT reject_revocation CHECK(revoked_at IS NULL)`)
	_, err = f.users.Password(ctx, u.ID, dto.UserPasswordRequest{Password: "replacement password", ExpectedRevision: 1})
	if err == nil {
		t.Fatal("expected revoke failure")
	}
	current, err := f.users.Get(ctx, u.ID)
	if err != nil || current.Revision != 1 {
		t.Fatal("failed revoke committed user")
	}
	if _, err := f.service.CurrentSession(f.ctx, token); err != nil {
		t.Fatal("failed revoke invalidated session")
	}
	if _, _, err := f.service.Authenticate(f.ctx, u.Email, createInput().Password); err != nil {
		t.Fatal("failed revoke committed password")
	}
}

type gateTransactions struct {
	base          userservice.Transactions
	held, release chan struct{}
}

func (g gateTransactions) WithinTx(ctx context.Context, o *pgx.TxOptions, fn func(repository.DBTX) error) error {
	return g.base.WithinTx(ctx, o, func(tx repository.DBTX) error {
		if err := fn(tx); err != nil {
			return err
		}
		close(g.held)
		select {
		case <-g.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func waitDatabaseLock(t *testing.T, f *fixture, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE((SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1),false)`, pid).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("database operation never waited for lock")
}

func TestLoginWaitsForUserSecurityMutation(t *testing.T) {
	for _, kind := range []string{"disable", "password", "role"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			ctx := f.actor()
			u, err := f.users.Create(ctx, createInput())
			if err != nil {
				t.Fatal(err)
			}
			var hash string
			f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash)
			gate := gateTransactions{
				base:    repository.NewRepository(f.pool, zap.NewNop()),
				held:    make(chan struct{}),
				release: make(chan struct{}),
			}
			defer func() {
				select {
				case <-gate.release:
				default:
					close(gate.release)
				}
			}()
			svc := userservice.NewService(
				userrepository.NewRepository(),
				gate,
				repository.NewRepository(f.pool, zap.NewNop()),
			)
			changed := make(chan error, 1)
			go func() {
				var err error
				switch kind {
				case "disable":
					_, err = svc.Update(ctx, u.ID, dto.UpdateUserRequest{IsActive: ptr(false), ExpectedRevision: 1})
				case "role":
					_, err = svc.Update(ctx, u.ID, dto.UpdateUserRequest{Role: ptr("storekeeper"), ExpectedRevision: 1})
				case "password":
					_, err = svc.Password(
						ctx,
						u.ID,
						dto.UserPasswordRequest{Password: "changed password", ExpectedRevision: 1},
					)
				}
				changed <- err
			}()
			select {
			case <-gate.held:
			case err := <-changed:
				t.Fatal("mutation failed", err)
			case <-f.ctx.Done():
				t.Fatal("mutation timeout")
			}
			conn, err := f.pool.Acquire(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			var pid int
			conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
			issued := make(chan error, 1)
			go func() {
				s, err := authrepository.NewRepository(conn).
					CreateSession(ctx, u.ID, hash, authservice.SessionTokenHash(uuid.NewString()), "csrf")
				if err == nil && s.User.Role != "storekeeper" {
					err = errors.New("stale role in new session")
				}
				issued <- err
			}()
			waitDatabaseLock(t, f, pid)
			close(gate.release)
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			err = <-issued
			if kind == "role" {
				if err != nil {
					t.Fatal("new role login", err)
				}
			} else if !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Fatal("stale login succeeded", err)
			}
		})
	}
}

func TestStaleAdminAfterAdministrationWait(t *testing.T) {
	f := newFixture(t)
	other, err := f.users.Create(
		f.actor(),
		dto.CreateUserRequest{Name: "Other", Email: "other@example.test", Role: "admin", Password: f.password},
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(f.ctx, `SELECT pg_advisory_xact_lock(160016,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, `UPDATE users SET role='manager' WHERE id=$1`, f.user); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.users.Update(
			f.actor(),
			other.ID,
			dto.UpdateUserRequest{Name: ptr("Unauthorized"), ExpectedRevision: 1},
		)
		result <- err
	}()
	// Find the waiting administration transaction, not an arbitrary sleep.
	deadline := time.Now().Add(5 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event='advisory' AND query LIKE '%160016%')`).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("admin mutation did not wait")
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("stale admin mutated", err)
	}
	var name string
	f.pool.QueryRow(f.ctx, `SELECT name FROM users WHERE id=$1`, other.ID).Scan(&name)
	if strings.Contains(name, "Unauthorized") {
		t.Fatal("unauthorized change persisted")
	}
}
