package dish_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	dishrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dish"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	dishservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dish"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	"go.uber.org/zap"
)

func orderBody(d uuid.UUID, version int) []byte {
	data, _ := json.Marshal(dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: d.String(), Portions: 1, RecipeVersion: &version}}})
	return data
}

func TestDishEditPreservesRealHistoricalOrderAndRefusesStaleRecipe(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Old ingredient", "g", true)
	p2 := f.product(t, "New ingredient", "ml", true)
	f.batch(t, p, "10.000", time.Now().Add(time.Hour))
	f.batch(t, p2, "10.000", time.Now().Add(time.Hour))
	d := f.createDish(t, p)
	base := repository.NewRepository(f.pool, zap.NewNop())
	repo := orderrepository.NewRepository()
	create := orderservice.NewCreateService(repo, base, idempotency.NewService(base))
	history := orderservice.NewHistoryService(repo, base)
	response, err := create.CreateIdempotent(f.actor(), f.user, uuid.New(), orderBody(d.ID, 1))
	if err != nil || response.Status != 201 {
		t.Fatalf("order: %d %v %s", response.Status, err, response.Body)
	}
	var order dto.OrderDetail
	if err := json.Unmarshal(response.Body, &order); err != nil {
		t.Fatal(err)
	}
	old, err := history.GetOrder(f.actor(), order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var linked uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT recipe_version_id FROM order_items WHERE order_id=$1`, order.ID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	next, err := f.dishes.Update(f.actor(), d.ID, dto.UpdateDishRequest{ExpectedRevision: 1, Name: ptr("New name"), Recipe: ptr(recipe(p2, "2"))})
	if err != nil || *next.RecipeVersion != 2 {
		t.Fatal(err)
	}
	if _, err := f.catalog.UpdateProduct(f.actor(), p, dto.UpdateProductRequest{ExpectedRevision: 1, Name: ptr("Renamed historical product"), IsActive: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	got, err := history.GetOrder(f.actor(), order.ID)
	if err != nil || !reflect.DeepEqual(old, got) {
		t.Fatal("historical order changed", err)
	}
	if got.Items[0].DishName != "Original" || *got.Items[0].RecipeVersion != 1 || got.Items[0].Allocations[0].ProductID != p || got.Items[0].Allocations[0].ProductName != "Old ingredient" || got.Items[0].Allocations[0].Quantity != "1.000" {
		t.Fatalf("history snapshots: %+v", got)
	}
	var oldRecipeProduct uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT product_id FROM recipe_items WHERE recipe_version_id=$1`, linked).Scan(&oldRecipeProduct); err != nil || oldRecipeProduct != p {
		t.Fatal("order recipe link changed")
	}
	movements := f.count(t, "stock_movements")
	key := uuid.New()
	response, err = create.CreateIdempotent(f.actor(), f.user, key, orderBody(d.ID, 1))
	if err != nil || response.Status != 409 || !strings.Contains(string(response.Body), "RECIPE_CHANGED") {
		t.Fatalf("stale recipe: %d %v %s", response.Status, err, response.Body)
	}
	if f.count(t, "orders") != 1 || f.count(t, "order_attempts") != 1 || f.count(t, "stock_movements") != movements {
		t.Fatal("stale recipe consumed stock")
	}
	response, err = create.CreateIdempotent(f.actor(), f.user, key, orderBody(d.ID, 1))
	if err != nil || response.Status != 409 || f.count(t, "order_attempts") != 1 {
		t.Fatal("stale replay added attempt")
	}
	response, err = create.CreateIdempotent(f.actor(), f.user, uuid.New(), orderBody(d.ID, 2))
	if err != nil || response.Status != 201 {
		t.Fatalf("new recipe order: %d %v", response.Status, err)
	}
	if err := json.Unmarshal(response.Body, &order); err != nil || order.Items[0].DishName != "New name" || order.Items[0].Allocations[0].ProductID != p2 || order.Items[0].Allocations[0].Quantity != "2.000" {
		t.Fatal("new order did not use new recipe")
	}
}

type pausedRecipe struct {
	*dishrepository.Repository
	inserted chan struct{}
	release  chan struct{}
}

func (p pausedRecipe) AddRecipe(ctx context.Context, tx repository.DBTX, dishID, actorID uuid.UUID, recipe []model.RecipeIngredient) error {
	if err := p.Repository.AddRecipe(ctx, tx, dishID, actorID, recipe); err != nil {
		return err
	}
	close(p.inserted)
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestOrderWaitsForRecipeSaveThenRechecksLatestVersion(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	d := f.createDish(t, p)
	f.batch(t, p, "10.000", time.Now().Add(time.Hour))
	base := repository.NewRepository(f.pool, zap.NewNop())
	inserted, release := make(chan struct{}), make(chan struct{})
	s := dishservice.NewService(pausedRecipe{dishrepository.NewRepository(), inserted, release}, menurepository.NewRepository(), base, base)
	saved := make(chan error, 1)
	go func() {
		_, err := s.Update(f.actor(), d.ID, dto.UpdateDishRequest{ExpectedRevision: 1, Recipe: ptr(recipe(p, "2"))})
		saved <- err
	}()
	select {
	case <-inserted:
	case <-f.ctx.Done():
		t.Fatal("recipe save did not reach pause")
	}
	create := orderservice.NewCreateService(orderrepository.NewRepository(), base, idempotency.NewService(base))
	result := make(chan model.IdempotencyResponse, 1)
	failure := make(chan error, 1)
	go func() {
		response, err := create.CreateIdempotent(f.actor(), f.user, uuid.New(), orderBody(d.ID, 1))
		result <- response
		failure <- err
	}()
	waitLock(t, f, "%FROM dishes%FOR UPDATE%")
	close(release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if err := <-failure; err != nil {
		t.Fatal(err)
	}
	response := <-result
	if response.Status != 409 || !strings.Contains(string(response.Body), "RECIPE_CHANGED") || f.count(t, "orders") != 0 || f.count(t, "stock_movements") != 0 {
		t.Fatalf("stale version after wait: %d %s", response.Status, response.Body)
	}
}

func TestRecipeSaveHoldsActorAndProductGuardsUntilCommit(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	base := repository.NewRepository(f.pool, zap.NewNop())
	inserted, release := make(chan struct{}), make(chan struct{})
	s := dishservice.NewService(pausedRecipe{dishrepository.NewRepository(), inserted, release}, menurepository.NewRepository(), base, base)
	saved := make(chan error, 1)
	go func() {
		_, err := s.Create(f.actor(), dto.CreateDishRequest{Name: "New active dish", Recipe: recipe(p, "1")})
		saved <- err
	}()
	select {
	case <-inserted:
	case <-f.ctx.Done():
		t.Fatal("save did not reach pause")
	}
	productResult := make(chan error, 1)
	go func() {
		_, err := f.catalog.UpdateProduct(f.actor(), p, dto.UpdateProductRequest{ExpectedRevision: 1, IsActive: ptr(false)})
		productResult <- err
	}()
	waitLock(t, f, "%FROM products%FOR UPDATE%")
	securityResult := make(chan error, 1)
	go func() {
		_, err := f.pool.Exec(f.ctx, `UPDATE users SET role='manager' WHERE id=$1`, f.user)
		securityResult <- err
	}()
	waitLock(t, f, "%UPDATE users SET role%")
	close(release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	var inUse *apperrors.ProductInUse
	if err := <-productResult; !errors.As(err, &inUse) || len(inUse.Dishes) != 1 {
		t.Fatalf("catalog missed committed active recipe: %v", err)
	}
	if err := <-securityResult; err != nil {
		t.Fatal(err)
	}
	if f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 || f.count(t, "audit_log") != 1 || f.count(t, "outbox_events") != 1 {
		t.Fatal("atomic save or guards")
	}
}
