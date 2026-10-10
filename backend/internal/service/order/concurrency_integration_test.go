package order

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
)

func TestOrderCreateIntegrationConcurrentOrdersShareProduct(t *testing.T) {
	f := newCreationFixture(t)
	product := f.product(t, "Shared")
	f.batch(t, product, "1.000", f.now.Add(time.Hour))
	dishes := []uuid.UUID{f.dish(t, "One", true), f.dish(t, "Two", true)}
	for _, dish := range dishes {
		f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	}
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, dish := range dishes {
		body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
		go func() { <-start; results <- f.createRequest(body, uuid.New()) }()
	}
	close(start)
	statuses := map[int]int{}
	for range dishes {
		response := <-results
		statuses[response.Code]++
		if response.Code == http.StatusConflict && !strings.Contains(response.Body.String(), "INSUFFICIENT_STOCK") {
			t.Fatalf("unexpected conflict: %s", response.Body.String())
		}
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusConflict] != 1 {
		t.Fatalf("concurrent statuses: %+v", statuses)
	}
	var remaining string
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE product_id=$1`, product).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != "0.000" || f.count(t, "orders") != 1 || f.count(t, "stock_movements") != 1 || f.count(t, "order_attempts") != 1 || f.count(t, "idempotency_keys") != 2 {
		t.Fatal("concurrent orders spent the same balance or lost refusal facts")
	}
}

func TestOrderCreateIntegrationOppositeBasketOrder(t *testing.T) {
	f := newCreationFixture(t)
	first, second := f.product(t, "First"), f.product(t, "Second")
	f.batch(t, first, "2.000", f.now.Add(time.Hour))
	f.batch(t, second, "2.000", f.now.Add(time.Hour))
	d1, d2 := f.dish(t, "One", true), f.dish(t, "Two", true)
	f.recipe(t, d1, 1, map[uuid.UUID]string{first: "1.000"})
	f.recipe(t, d2, 1, map[uuid.UUID]string{second: "1.000"})
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, dishes := range [][]uuid.UUID{{d1, d2}, {d2, d1}} {
		body := orderBody(t, dto.OrderRequestItem{DishID: dishes[0].String(), Portions: 1}, dto.OrderRequestItem{DishID: dishes[1].String(), Portions: 1})
		go func() { <-start; results <- f.createRequest(body, uuid.New()) }()
	}
	close(start)
	for range 2 {
		response := <-results
		if response.Code != http.StatusCreated {
			t.Fatalf("opposite baskets: %d %s", response.Code, response.Body.String())
		}
	}
	if f.count(t, "orders") != 2 || f.count(t, "stock_movements") != 4 {
		t.Fatal("concurrent basket orders missing")
	}
}

type signaledOrderRepository struct {
	*orderrepository.Repository
	entered  chan struct{}
	products bool
}

func (r *signaledOrderRepository) LockDishes(ctx context.Context, tx repository.DBTX, ids []uuid.UUID) error {
	if !r.products {
		close(r.entered)
	}
	return r.Repository.LockDishes(ctx, tx, ids)
}

func (r *signaledOrderRepository) LockProducts(ctx context.Context, tx repository.DBTX, ids []uuid.UUID) error {
	if r.products {
		close(r.entered)
	}
	return r.Repository.LockProducts(ctx, tx, ids)
}

func TestOrderCreateIntegrationInFlightKeyThenExactReplay(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Original dish", true)
	f.batch(t, product, "2.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	key := uuid.New()
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(f.ctx)
	if _, err := holder.Exec(f.ctx, `SELECT id FROM dishes WHERE id=$1 FOR UPDATE`, dish); err != nil {
		t.Fatal(err)
	}
	probe := &signaledOrderRepository{Repository: orderrepository.NewRepository(), entered: make(chan struct{})}
	f.create.repo = probe
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- f.createRequest(body, key) }()
	select {
	case <-probe.entered:
	case <-f.ctx.Done():
		t.Fatal("order did not acquire idempotency lock")
	}
	inFlight := f.createRequest(body, key)
	if inFlight.Code != http.StatusConflict || !strings.Contains(inFlight.Body.String(), "REQUEST_IN_PROGRESS") || inFlight.Header().Get("Retry-After") != "1" {
		t.Fatalf("in-flight key: %d %s", inFlight.Code, inFlight.Body.String())
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	first := <-finished
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	f.exec(t, `UPDATE dishes SET name='Renamed',is_active=false WHERE id=$1`, dish)
	f.recipe(t, dish, 2, map[uuid.UUID]string{product: "2.000"})
	before := f.snapshot(t)
	repeat := f.createRequest(body, key)
	if repeat.Code != first.Code || !bytes.Equal(repeat.Body.Bytes(), first.Body.Bytes()) || repeat.Header().Get("Location") != first.Header().Get("Location") || f.snapshot(t) != before {
		t.Fatal("completed order was recomputed or rewritten on replay")
	}
	conflict := f.createRequest(body+" ", key)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "IDEMPOTENCY_CONFLICT") || f.snapshot(t) != before {
		t.Fatal("changed raw body did not conflict")
	}
}

func TestOrderCreateIntegrationExpiryAfterProductWait(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.batch(t, product, "1.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(f.ctx)
	if _, err := holder.Exec(f.ctx, `SELECT id FROM products WHERE id=$1 FOR UPDATE`, product); err != nil {
		t.Fatal(err)
	}
	probe := &signaledOrderRepository{Repository: orderrepository.NewRepository(), entered: make(chan struct{}), products: true}
	f.create.repo = probe
	clock := make(chan time.Time, 1)
	f.create.now = func() time.Time { return <-clock }
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- f.createRequest(body, uuid.New()) }()
	select {
	case <-probe.entered:
	case <-f.ctx.Done():
		t.Fatal("order did not reach product lock")
	}
	clock <- f.now.Add(2 * time.Hour)
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	response := <-finished
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "only_expired_stock") || f.count(t, "orders") != 0 || f.count(t, "stock_movements") != 0 {
		t.Fatalf("expiry after wait: %d %s", response.Code, response.Body.String())
	}
}

func TestOrderCreateIntegrationRecipeChangeAfterDishWait(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.batch(t, product, "5.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(f.ctx)
	if _, err := holder.Exec(f.ctx, `SELECT id FROM dishes WHERE id=$1 FOR UPDATE`, dish); err != nil {
		t.Fatal(err)
	}
	probe := &signaledOrderRepository{Repository: orderrepository.NewRepository(), entered: make(chan struct{})}
	f.create.repo = probe
	version := 1
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1, RecipeVersion: &version})
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- f.createRequest(body, uuid.New()) }()
	select {
	case <-probe.entered:
	case <-f.ctx.Done():
		t.Fatal("order did not reach dish lock")
	}
	recipe := uuid.New()
	if _, err := holder.Exec(f.ctx, `INSERT INTO recipe_versions(id,dish_id,version,created_by)VALUES($1,$2,2,$3)`, recipe, dish, f.actor); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(f.ctx, `INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion)VALUES($1,$2,'2.000')`, recipe, product); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	response := <-finished
	var refusal dto.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusConflict || refusal.Error.Code != "RECIPE_CHANGED" || f.count(t, "orders") != 0 || f.count(t, "stock_movements") != 0 || f.count(t, "order_attempts") != 1 {
		t.Fatalf("recipe changed after wait: %d %s", response.Code, response.Body.String())
	}
}
