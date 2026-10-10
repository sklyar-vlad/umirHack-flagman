package order

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestOrderCreateIntegrationRollsBackEveryWriteFailure(t *testing.T) {
	for _, table := range []string{"orders", "order_items", "batches", "stock_movements", "audit_log", "outbox_events", "idempotency_keys", "order_attempts"} {
		t.Run(table, func(t *testing.T) {
			f := newCreationFixture(t)
			product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
			if table != "order_attempts" {
				f.batch(t, product, "2.000", f.now.Add(time.Hour))
			}
			f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
			before := f.snapshot(t)
			key := uuid.New()
			body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
			// Only this fixture's isolated schema is modified. NOT VALID permits
			// fixture rows but rejects the next write, even after earlier writes.
			f.exec(t, "ALTER TABLE "+table+" ADD CONSTRAINT test_reject_write CHECK(false) NOT VALID")
			response := f.createRequest(body, key)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("%s failure: %d %s", table, response.Code, response.Body.String())
			}
			if f.snapshot(t) != before {
				t.Fatalf("%s failure retained writes or a completed key", table)
			}
			f.exec(t, "ALTER TABLE "+table+" DROP CONSTRAINT test_reject_write")
			retry := f.createRequest(body, key)
			want := http.StatusCreated
			if table == "order_attempts" {
				want = http.StatusConflict
			}
			if retry.Code != want || f.count(t, "idempotency_keys") != 1 {
				t.Fatalf("retry after %s failure: %d %s", table, retry.Code, retry.Body.String())
			}
		})
	}
}

func TestOrderCreateIntegrationRollsBackCommitFailure(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.batch(t, product, "2.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	f.exec(t, `CREATE FUNCTION test_reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test commit failure'; END $$`)
	f.exec(t, `CREATE CONSTRAINT TRIGGER test_reject_commit AFTER INSERT ON idempotency_keys DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION test_reject_commit()`)
	before := f.snapshot(t)
	key := uuid.New()
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	response := f.createRequest(body, key)
	if response.Code != http.StatusInternalServerError || f.snapshot(t) != before {
		t.Fatalf("failed commit retained writes: %d %s", response.Code, response.Body.String())
	}
	f.exec(t, `DROP TRIGGER test_reject_commit ON idempotency_keys`)
	retry := f.createRequest(body, key)
	if retry.Code != http.StatusCreated || f.count(t, "orders") != 1 || f.count(t, "idempotency_keys") != 1 {
		t.Fatalf("failed commit key cannot retry: %d %s", retry.Code, retry.Body.String())
	}
}
