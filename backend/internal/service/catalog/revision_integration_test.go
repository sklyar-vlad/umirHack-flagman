package catalog_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
)

func TestConcurrentCatalogRevisionsAndHTTPConflict(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	for _, product := range []bool{false, true} {
		var wg sync.WaitGroup
		result := make(chan error, 2)
		for _, name := range []string{"First", "Second"} {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				var err error
				if product {
					_, err = f.catalog.UpdateProduct(
						f.actor(),
						p.ID,
						&dto.UpdateProductRequest{Name: &name, ExpectedRevision: 1},
					)
				} else {
					_, err = f.catalog.UpdateCategory(
						f.actor(),
						p.Category.ID,
						dto.UpdateCategoryRequest{Name: name, ExpectedRevision: 1},
					)
				}
				result <- err
			}(name)
		}
		wg.Wait()
		close(result)
		success, conflict := 0, 0
		for err := range result {
			if err == nil {
				success++
			} else {
				var rev *apperrors.RevisionConflict
				if !errors.As(err, &rev) || rev.CurrentRevision != 2 {
					t.Fatal(err)
				}
				conflict++
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal(success, conflict)
		}
	}
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/products/" + p.ID.String(), "/product-categories/" + p.Category.ID.String()} {
		w := request(
			f.router(),
			"PATCH",
			path,
			`{"name":"Stale","expected_revision":1}`,
			token,
			session.CSRFToken,
			"https://flagman.example",
		)
		if w.Code != 409 || !stringsContains(w.Body.String(), `REVISION_CONFLICT`, `"current_revision":2`) {
			t.Fatal(w.Code, w.Body)
		}
	}
	var audits int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_log`).Scan(&audits); err != nil || audits != 4 {
		t.Fatal("stale request audited", audits, err)
	}
}

func TestActorRightsRecheckedAfterSecurityLockWait(t *testing.T) {
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
				_, err := f.catalog.CreateCategory(f.actor(), dto.CreateCategoryRequest{Name: "Forbidden"})
				result <- err
			}()
			waitLock(t, f, "%FROM users%FOR SHARE%")
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatal("stale admin wrote", err)
			}
			var count int
			if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM product_categories)+(SELECT count(*) FROM audit_log)`).
				Scan(&count); err != nil ||
				count != 0 {
				t.Fatal("forbidden writes", err)
			}
		})
	}
}
