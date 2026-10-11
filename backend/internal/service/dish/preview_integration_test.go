package dish_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	dishrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dish"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	dishservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dish"
	menuservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	"go.uber.org/zap"
)

func (f *fixture) batch(t *testing.T, p uuid.UUID, amount string, expiry time.Time) uuid.UUID {
	t.Helper()
	s, b := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO supplies(id,number,received_at,created_by) VALUES($1,$2,now(),$3)`, s, s.String(), f.user)
	f.exec(t, `INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,1,$4,$4,'explicit',$5)`, b, s, p, amount, expiry)
	return b
}

func TestDishPreviewSharedAvailabilityRequirementsAndNoWrites(t *testing.T) {
	f := newFixture(t)
	x, y := f.product(t, "First", "g", true), f.product(t, "Second", "ml", true)
	valid := f.batch(t, x, "2.999", time.Now().Add(time.Hour))
	f.batch(t, x, "20.000", time.Now().Add(-time.Hour))
	expired := f.batch(t, y, "10.000", time.Now().Add(-time.Hour))
	input := append(recipe(x, "0.3"), recipe(y, "0.4")...)
	d, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: "Preview comparison", Recipe: input})
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{"dishes", "recipe_versions", "recipe_items", "orders", "order_items", "order_attempts", "stock_movements", "audit_log", "outbox_events", "idempotency_keys"}
	counts := make(map[string]int)
	for _, table := range tables {
		counts[table] = f.count(t, table)
	}
	result, err := f.dishes.Preview(f.actor(), dto.PreviewDishRequest{Recipe: input})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsAvailable || result.AvailablePortions != "0" || len(result.Requirements) != 2 || len(result.Shortages) != 1 || result.Shortages[0].ProductID != y || result.Shortages[0].ReasonCode != "only_expired_stock" || result.Shortages[0].ExpiredQuantity != "10.000" {
		t.Fatalf("preview: %+v", result)
	}
	if result.Requirements[0].RequiredQuantity != "0.300" || result.Requirements[0].AvailableQuantity != "2.999" || result.Requirements[0].MissingQuantity != "0.000" || result.Requirements[1].MissingQuantity != "0.400" {
		t.Fatalf("requirements: %+v", result.Requirements)
	}
	base := repository.NewRepository(f.pool, zap.NewNop())
	menu := menuservice.NewService(menurepository.NewRepository(), base)
	persisted, err := menu.GetDish(f.actor(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.IsAvailable != result.IsAvailable || persisted.AvailablePortions != result.AvailablePortions || !reflect.DeepEqual(persisted.Shortages, result.Shortages) {
		t.Fatal("preview diverged from S08")
	}
	if _, err := time.Parse(time.RFC3339Nano, result.Meta["server_time"].(string)); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if counts[table] != f.count(t, table) {
			t.Fatalf("preview wrote %s", table)
		}
	}
	var remaining string
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id=$1`, valid).Scan(&remaining); err != nil || remaining != "2.999" {
		t.Fatal("preview changed stock")
	}
	f.exec(t, `UPDATE batches SET expires_at=now()+interval '1 hour' WHERE id=$1`, expired)
	result, err = f.dishes.Preview(f.actor(), dto.PreviewDishRequest{Recipe: input})
	if err != nil || !result.IsAvailable || result.AvailablePortions != "9" || len(result.Shortages) != 0 {
		t.Fatalf("fresh expiry: %+v %v", result, err)
	}
	boundary, _ := time.Parse(time.RFC3339Nano, result.Meta["server_time"].(string))
	f.exec(t, `UPDATE batches SET expires_at=$2 WHERE id=$1`, valid, boundary)
	result, err = f.dishes.Preview(f.actor(), dto.PreviewDishRequest{Recipe: input})
	if err != nil || result.IsAvailable || len(result.Shortages) != 1 || result.Shortages[0].ExpiredQuantity != "22.999" {
		t.Fatalf("expiry boundary: %+v %v", result, err)
	}
}

type readTransactions struct {
	t    *testing.T
	base dishservice.Transactions
}

func (r readTransactions) WithinTx(ctx context.Context, options *pgx.TxOptions, fn func(repository.DBTX) error) error {
	if options.AccessMode != pgx.ReadOnly || options.IsoLevel != pgx.RepeatableRead {
		r.t.Fatal("preview transaction options")
	}
	return r.base.WithinTx(ctx, options, func(tx repository.DBTX) error {
		var readOnly, isolation string
		if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_read_only'),current_setting('transaction_isolation')`).Scan(&readOnly, &isolation); err != nil {
			return err
		}
		if readOnly != "on" || isolation != "repeatable read" {
			r.t.Fatal("preview transaction not protected")
		}
		return fn(tx)
	})
}

type concurrentProducts struct {
	*dishrepository.Repository
	f     *fixture
	t     *testing.T
	batch uuid.UUID
}

func (r concurrentProducts) Products(ctx context.Context, tx repository.DBTX, ids []uuid.UUID, now time.Time, lock bool) ([]model.RecipeProduct, error) {
	if lock {
		r.t.Fatal("preview locked catalog")
	}
	items, err := r.Repository.Products(ctx, tx, ids, now, false)
	if err != nil {
		return nil, err
	}
	r.f.exec(r.t, `UPDATE batches SET remaining_quantity=0 WHERE id=$1`, r.batch)
	return items, nil
}

func TestPreviewUsesProtectedSnapshotAndHTTPAdminPolicy(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	b := f.batch(t, p, "1.001", time.Now().Add(time.Hour))
	base := repository.NewRepository(f.pool, zap.NewNop())
	s := dishservice.NewService(concurrentProducts{dishrepository.NewRepository(), f, t, b}, menurepository.NewRepository(), readTransactions{t, base}, base)
	result, err := s.Preview(f.actor(), dto.PreviewDishRequest{Recipe: recipe(p, "0.001")})
	if err != nil || !result.IsAvailable || result.AvailablePortions != "1001" {
		t.Fatalf("snapshot: %+v %v", result, err)
	}
	result, err = f.dishes.Preview(f.actor(), dto.PreviewDishRequest{Recipe: recipe(p, "0.001")})
	if err != nil || result.IsAvailable {
		t.Fatal("new snapshot did not recalculate")
	}
	data, _ := json.Marshal(dto.PreviewDishRequest{Recipe: recipe(p, "1")})
	router := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "POST", "/dishes/preview", string(data), token, session.CSRFToken, "https://flagman.example")
		want := 403
		if role == "admin" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("preview by %s: %d %s", role, w.Code, w.Body.String())
		}
		if role == "admin" {
			if w := request(router, "POST", "/dishes/preview", string(data), token, "", ""); w.Code != 403 {
				t.Fatal("preview csrf bypass")
			}
			for _, body := range []string{`{"recipe":[]}`, `{"recipe":null}`, `{"recipe":[],"name":"x"}`, `{"recipe":[{"product_id":null}]}`} {
				if w := request(router, "POST", "/dishes/preview", body, token, session.CSRFToken, "https://flagman.example"); w.Code != 422 {
					t.Fatalf("preview shape: %d", w.Code)
				}
			}
		}
	}
}
