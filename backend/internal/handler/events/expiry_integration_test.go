package events

import (
	"bufio"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	offersrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	offersservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/hotoffers"
	menuservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	"go.uber.org/zap"
)

func TestTimeEventRefreshesRealMenuExpiryAndWarningWithoutBusinessWrites(t *testing.T) {
	f := newFixture(t)
	server, s, token, _ := f.stream(t, "manager")
	f.exec(t, `UPDATE restaurant_settings SET expiring_threshold_hours=1,auto_writeoff_expired=false`)
	category, supply := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Food')`, category)
	f.exec(
		t,
		`INSERT INTO supplies(id,number,received_at,created_by) SELECT $1,'initial',clock_timestamp(),id FROM users`,
		supply,
	)
	dishes := []uuid.UUID{}
	for i, interval := range []string{"2 seconds", "1 hour 2 seconds"} {
		product, batch, dish, recipe := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		dishes = append(dishes, dish)
		f.exec(
			t,
			`INSERT INTO products(id,category_id,name,unit,display_unit) VALUES($1,$2,$3,'g','g')`,
			product,
			category,
			product.String(),
		)
		f.exec(
			t,
			`INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES($1,$2,$3,$4,100,100,'explicit',clock_timestamp()+$5::interval)`,
			batch,
			supply,
			product,
			i+1,
			interval,
		)
		f.exec(t, `INSERT INTO dishes(id,name) VALUES($1,$2)`, dish, dish.String())
		f.exec(
			t,
			`INSERT INTO recipe_versions(id,dish_id,version,created_by) SELECT $1,$2,1,id FROM users`,
			recipe,
			dish,
		)
		f.exec(
			t,
			`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,10)`,
			recipe,
			product,
		)
	}
	transactions := repository.NewRepository(f.pool, zap.NewNop())
	menu := menuservice.NewService(menurepo.NewRepository(), transactions)
	offers := offersservice.NewService(offersrepo.NewRepository(), menurepo.NewRepository(), transactions)
	first, err := menu.GetDish(f.ctx, dishes[0])
	if err != nil || !first.Availability.IsAvailable {
		t.Fatal(first, err)
	}
	before, err := offers.List(f.ctx, model.HotOffersFilter{Page: 1, PageSize: 10})
	if err != nil || len(before.Items) != 1 || before.Items[0].Dish.ID != dishes[0] {
		t.Fatal(before, err)
	}
	response := open(t, server, token, "")
	defer response.Body.Close()
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	defer func() { cancel(); <-done }()
	r := bufio.NewReader(response.Body)
	firstEvent := next(t, r)
	if firstEvent.Type != "inventory.time_changed" {
		t.Fatal(firstEvent)
	}
	secondEvent := next(t, r) // Actual next 30-second timer; heartbeat is a comment.
	if secondEvent.Type != "inventory.time_changed" || secondEvent.ID <= firstEvent.ID ||
		len(secondEvent.Payload.ProductIDs) != 0 {
		t.Fatal(secondEvent)
	}
	after, err := menu.GetDish(f.ctx, dishes[0])
	if err != nil || after.Availability.IsAvailable {
		t.Fatal(after, err)
	}
	// The other batch entered the 1-hour warning threshold without a mutation.
	nowOffers, err := offers.List(f.ctx, model.HotOffersFilter{Page: 1, PageSize: 10})
	if err != nil || len(nowOffers.Items) != 1 || nowOffers.Items[0].Dish.ID != dishes[1] {
		t.Fatal(nowOffers, err)
	}
	var unchanged bool
	if err := f.pool.QueryRow(f.ctx, `SELECT
 (SELECT count(*) FROM supplies)=1 AND (SELECT count(*) FROM batches WHERE remaining_quantity=100)=2
 AND (SELECT count(*) FROM recipe_versions)=2 AND (SELECT count(*) FROM recipe_items)=2
 AND NOT EXISTS(SELECT 1 FROM orders) AND NOT EXISTS(SELECT 1 FROM order_attempts)
 AND NOT EXISTS(SELECT 1 FROM stock_movements) AND NOT EXISTS(SELECT 1 FROM audit_log)
 AND (SELECT count(*) FROM outbox_events)=2`).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("time invalidation changed business facts", err)
	}
	if secondEvent.Payload.OccurredAt.Sub(firstEvent.Payload.OccurredAt) < 29*time.Second {
		t.Fatal("time-event cadence")
	}
}
