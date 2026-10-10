package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions, now: time.Now}
}

// Check reads recipes and stock from one consistent snapshot. It neither
// reserves ingredients nor records a successful or refused order.
func (s *Service) Check(ctx context.Context, request dto.OrderRequest) (dto.OrderCheck, error) {
	items, err := normalizeBasket(request)
	if err != nil {
		return dto.OrderCheck{}, err
	}
	ids := make([]uuid.UUID, len(items))
	for i, item := range items {
		ids[i] = item.dishID
	}
	var dishes []model.Dish
	var now time.Time
	err = s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			now = s.now().UTC()
			var err error
			dishes, err = s.repo.Load(ctx, tx, &model.DishFilter{IDs: ids}, now)
			return err
		},
	)
	if err != nil {
		return dto.OrderCheck{}, err
	}
	byID := make(map[uuid.UUID]model.Dish, len(dishes))
	for _, dish := range dishes {
		byID[dish.ID] = dish
	}
	for _, item := range items {
		if _, exists := byID[item.dishID]; !exists {
			return dto.OrderCheck{}, &validations.FieldError{
				Field: fmt.Sprintf("items[%d].dish_id", item.inputIndex), Message: "Блюдо не найдено",
			}
		}
	}
	result, err := checkBasket(items, byID)
	if err != nil {
		return dto.OrderCheck{}, err
	}
	result.Meta["server_time"] = now.Format(time.RFC3339Nano)
	return result, nil
}
