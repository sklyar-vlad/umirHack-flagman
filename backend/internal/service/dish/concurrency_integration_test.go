package dish_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
)

func waitLock(t *testing.T, f *fixture, pattern string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE $2)`, f.schema, pattern).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("operation did not wait for expected lock")
}

func TestConcurrentDishSavesCreateOneVersionAndOneRevisionConflict(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	d := f.createDish(t, p)
	var wg sync.WaitGroup
	result := make(chan error, 2)
	for _, quantity := range []string{"2", "3"} {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			_, err := f.dishes.Update(
				f.actor(),
				d.ID,
				dto.UpdateDishRequest{ExpectedRevision: 1, Name: ptr("Name " + q), Recipe: ptr(recipe(p, q))},
			)
			result <- err
		}(quantity)
	}
	wg.Wait()
	close(result)
	success, conflicts := 0, 0
	for err := range result {
		if err == nil {
			success++
		} else {
			var conflict *apperrors.RevisionConflict
			if !errors.As(err, &conflict) || conflict.CurrentRevision != 2 {
				t.Fatal(err)
			}
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 || f.count(t, "recipe_versions") != 2 || f.count(t, "recipe_items") != 2 ||
		f.count(t, "audit_log") != 2 ||
		f.count(t, "outbox_events") != 2 {
		t.Fatal("concurrent edit produced extra writes")
	}
	var name, quantity string
	if err := f.pool.QueryRow(f.ctx, `SELECT d.name,ri.quantity_per_portion::text FROM dishes d JOIN recipe_versions rv ON rv.dish_id=d.id AND rv.version=2 JOIN recipe_items ri ON ri.recipe_version_id=rv.id WHERE d.id=$1`, d.ID).
		Scan(&name, &quantity); err != nil {
		t.Fatal(err)
	}
	if (name != "Name 2" || quantity != "2.000") && (name != "Name 3" || quantity != "3.000") {
		t.Fatal("metadata and recipe saved from different edits")
	}
}

func TestRecipeProductActivityAndUnitsRecheckedAfterLockWait(t *testing.T) {
	for _, change := range []string{"is_active=false", "unit='pcs',display_unit='pcs'"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			p := f.product(t, "Ingredient", "g", true)
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.ctx)
			if _, err := tx.Exec(f.ctx, `UPDATE products SET `+change+` WHERE id=$1`, p); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: "Waiting", Recipe: recipe(p, "0.001")})
				result <- err
			}()
			waitLock(t, f, "%FROM products%FOR SHARE%")
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; err == nil {
				t.Fatal("stale catalog recipe saved")
			}
			if f.count(t, "dishes") != 0 || f.count(t, "recipe_versions") != 0 || f.count(t, "audit_log") != 0 ||
				f.count(t, "outbox_events") != 0 {
				t.Fatal("stale recipe left writes")
			}
		})
	}
}

func TestDishActorRightsRecheckedAfterSecurityWait(t *testing.T) {
	for _, change := range []string{"role='manager'", "is_active=false"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			p := f.product(t, "Ingredient", "g", true)
			d := f.createDish(t, p)
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
				_, err := f.dishes.Update(
					f.actor(),
					d.ID,
					dto.UpdateDishRequest{ExpectedRevision: 1, Recipe: ptr(recipe(p, "2"))},
				)
				result <- err
			}()
			waitLock(t, f, "%FROM users%FOR SHARE%")
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatalf("stale actor wrote: %v", err)
			}
			if f.count(t, "recipe_versions") != 1 || f.count(t, "audit_log") != 1 || f.count(t, "outbox_events") != 1 {
				t.Fatal("forbidden writes survived")
			}
		})
	}
}
