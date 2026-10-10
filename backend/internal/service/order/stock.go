package order

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type StockRepository interface {
	Repository
	LockDishes(context.Context, repository.DBTX, []uuid.UUID) error
	LockProducts(context.Context, repository.DBTX, []uuid.UUID) error
	LockBatches(context.Context, repository.DBTX, []uuid.UUID) ([]model.OrderBatch, error)
}

type lockedStock struct {
	dishes     map[uuid.UUID]model.Dish
	batches    []model.OrderBatch
	serverTime time.Time
}

func lockBasket(
	ctx context.Context,
	repo StockRepository,
	tx repository.DBTX,
	items []basketItem,
	now func() time.Time,
) (lockedStock, error) {
	ids := make([]uuid.UUID, len(items))
	for i, item := range items {
		ids[i] = item.dishID
	}
	if err := repo.LockDishes(ctx, tx, ids); err != nil {
		return lockedStock{}, err
	}
	// Dish locks stabilize the latest recipe while discovering product locks.
	dishes, err := repo.Load(ctx, tx, &model.DishFilter{IDs: ids}, time.Time{})
	if err != nil {
		return lockedStock{}, err
	}
	known := make(map[uuid.UUID]bool, len(dishes))
	products := []uuid.UUID{}
	for _, dish := range dishes {
		known[dish.ID] = true
		for _, ingredient := range dish.Recipe {
			products = append(products, ingredient.ProductID)
		}
	}
	for _, item := range items {
		if !known[item.dishID] {
			return lockedStock{}, &validations.FieldError{
				Field:   fmt.Sprintf("items[%d].dish_id", item.inputIndex),
				Message: "Блюдо не найдено",
			}
		}
	}
	slices.SortFunc(products, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	products = slices.Compact(products)
	if err := repo.LockProducts(ctx, tx, products); err != nil {
		return lockedStock{}, err
	}
	batches, err := repo.LockBatches(ctx, tx, products)
	if err != nil {
		return lockedStock{}, err
	}
	serverTime := now().UTC()
	dishes, err = repo.Load(ctx, tx, &model.DishFilter{IDs: ids}, serverTime)
	if err != nil {
		return lockedStock{}, err
	}
	state := lockedStock{dishes: make(map[uuid.UUID]model.Dish, len(dishes)), batches: batches, serverTime: serverTime}
	for _, dish := range dishes {
		state.dishes[dish.ID] = dish
	}
	return state, nil
}
