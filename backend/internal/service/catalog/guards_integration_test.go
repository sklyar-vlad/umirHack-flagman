package catalog_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func (f *fixture) recipe(t *testing.T, dish uuid.UUID, version int, product uuid.UUID) {
	t.Helper()
	v := uuid.New()
	f.exec(
		t,
		`INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,$3,$4)`,
		v,
		dish,
		version,
		f.user,
	)
	f.exec(t, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,1)`, v, product)
}

func (f *fixture) batch(t *testing.T, product uuid.UUID) uuid.UUID {
	t.Helper()
	s, b := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by) VALUES($1,$1::uuid::text,now(),$2)`, s, f.user)
	f.exec(
		t,
		`INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,1,0,'explicit',now()-interval '1 day')`,
		b,
		s,
		product,
	)
	return b
}

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
	t.Fatal("operation did not wait for expected database lock")
}

func (f *fixture) supplies() *supplyservice.Service {
	base := repository.NewRepository(f.pool, zap.NewNop())
	return supplyservice.NewService(supplyrepository.NewRepository(), base, nil)
}

func TestHistoricalUnitGuardsAndDisplayEdits(t *testing.T) {
	for _, history := range []string{"none", "depleted expired batch", "old recipe of inactive dish"} {
		t.Run(history, func(t *testing.T) {
			f := newFixture(t)
			p := f.product(t)
			if history == "depleted expired batch" {
				f.batch(t, p.ID)
			}
			if history == "old recipe of inactive dish" {
				d := uuid.New()
				f.exec(t, `INSERT INTO dishes(id,name,is_active) VALUES($1,'Old dish',false)`, d)
				f.recipe(t, d, 1, p.ID)
				f.exec(t, `INSERT INTO recipe_versions(dish_id,version,created_by) VALUES($1,2,$2)`, d, f.user)
			}
			_, err := f.catalog.UpdateProduct(
				f.actor(),
				p.ID,
				&dto.UpdateProductRequest{Unit: ptr("g"), DisplayUnit: ptr("kg"), ExpectedRevision: 1},
			)
			rev := 1
			if history == "none" {
				if err != nil {
					t.Fatal(err)
				}
				rev = 2
			} else if !errors.Is(err, apperrors.ErrUnitLocked) {
				t.Fatal("unit changed with history", err)
			}
			display := "ml"
			if history == "none" {
				display = "g"
			}
			got, err := f.catalog.UpdateProduct(
				f.actor(),
				p.ID,
				&dto.UpdateProductRequest{DisplayUnit: &display, ExpectedRevision: rev},
			)
			if err != nil || got.DisplayUnit != display {
				t.Fatal("display change refused", err)
			}
		})
	}
}

func TestDeactivateCurrentIngredientsAndPreserveHistoryAndBalances(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	b := f.batch(t, p.ID)
	f.exec(t, `UPDATE batches SET remaining_quantity=1 WHERE id=$1`, b)
	a, z, inactive, old := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, d := range []struct {
		id     uuid.UUID
		name   string
		active bool
	}{{z, "Z dish", true}, {a, "A dish", true}, {inactive, "Inactive", false}, {old, "Old", true}} {
		f.exec(t, `INSERT INTO dishes(id,name,is_active) VALUES($1,$2,$3)`, d.id, d.name, d.active)
		f.recipe(t, d.id, 1, p.ID)
	}
	f.exec(t, `INSERT INTO recipe_versions(dish_id,version,created_by) VALUES($1,2,$2)`, old, f.user)
	_, err := f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{IsActive: ptr(false), ExpectedRevision: 1},
	)
	var inUse *apperrors.ProductInUse
	if !errors.As(err, &inUse) || len(inUse.Dishes) != 2 || inUse.Dishes[0].ID != a || inUse.Dishes[1].ID != z {
		t.Fatal("incorrect active/latest guards", err)
	}
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	w := request(
		f.router(),
		"PATCH",
		"/products/"+p.ID.String(),
		`{"is_active":false,"expected_revision":1}`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 409 || !stringsContains(w.Body.String(), "PRODUCT_IN_USE", a.String(), z.String()) {
		t.Fatal(w.Code, w.Body)
	}
	f.exec(t, `UPDATE dishes SET is_active=false WHERE id IN($1,$2)`, a, z)
	got, err := f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{IsActive: ptr(false), ExpectedRevision: 1},
	)
	if err != nil || got.IsActive {
		t.Fatal(got, err)
	}
	var balance string
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text,(SELECT count(*) FROM recipe_items WHERE product_id=$2) FROM batches WHERE id=$1`, b, p.ID).
		Scan(&balance, &count); err != nil ||
		balance != "1.000" ||
		count != 4 {
		t.Fatal("deactivation changed stock/history", balance, count, err)
	}
	_, err = f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{Unit: ptr("g"), DisplayUnit: ptr("g"), ExpectedRevision: 2},
	)
	if !errors.Is(err, apperrors.ErrUnitLocked) {
		t.Fatal(err)
	}
	w = request(
		f.router(),
		"PATCH",
		"/products/"+p.ID.String(),
		`{"unit":"g","display_unit":"g","expected_revision":2}`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 409 || !stringsContains(w.Body.String(), "UNIT_LOCKED") {
		t.Fatal(w.Code, w.Body)
	}
}

func stringsContains(raw string, values ...string) bool {
	for _, v := range values {
		if !strings.Contains(raw, v) {
			return false
		}
	}
	return true
}

func TestUnitGuardRechecksHistoryAfterReceiptLockWait(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err := tx.Exec(f.ctx, `SELECT id FROM products WHERE id=$1 FOR UPDATE`, p.ID); err != nil {
		t.Fatal(err)
	}
	s, b := uuid.New(), uuid.New()
	if _, err := tx.Exec(
		f.ctx,
		`INSERT INTO supplies(id,number,received_at,created_by) VALUES($1,'Waiting supply',now(),$2)`,
		s,
		f.user,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(
		f.ctx,
		`INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,1,1,'explicit',now()+interval '1 day')`,
		b,
		s,
		p.ID,
	); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.catalog.UpdateProduct(
			f.actor(),
			p.ID,
			&dto.UpdateProductRequest{Unit: ptr("g"), DisplayUnit: ptr("g"), ExpectedRevision: 1},
		)
		result <- err
	}()
	waitLock(t, f, "%FOR UPDATE OF p%")
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, apperrors.ErrUnitLocked) {
		t.Fatal("history ignored after wait", err)
	}
}

func TestReceiptAfterDeactivateWaitRechecksActivity(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err := tx.Exec(f.ctx, `UPDATE products SET is_active=false WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.supplies().
			Create(f.ctx, f.user, dto.CreateSupplyRequest{Number: "Waiting", ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano), Items: []dto.CreateSupplyItem{{ProductID: p.ID.String(), Quantity: "1", ExpiresAt: ptr(time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano))}}})
		result <- err
	}()
	waitLock(t, f, "%FROM products%FOR UPDATE%")
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil {
		t.Fatal("receipt accepted inactive product")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM batches`).Scan(&count); err != nil || count != 0 {
		t.Fatal("receipt wrote stock", err)
	}
}
