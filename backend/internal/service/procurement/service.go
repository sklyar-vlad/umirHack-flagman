package procurement

import (
	"cmp"
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Load(context.Context, repository.DBTX, model.ProcurementFilter, time.Time) ([]model.ProcurementStock, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type MenuRepository interface {
	Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error)
}

type Service struct {
	repo         Repository
	menu         MenuRepository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, menuRepo MenuRepository, transactions Transactions) *Service {
	return &Service{repo: repo, menu: menuRepo, transactions: transactions, now: time.Now}
}

func (s *Service) List(ctx context.Context, filter model.ProcurementFilter) (dto.ProcurementListResponse, error) {
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return dto.ProcurementListResponse{}, err
	}
	var stock []model.ProcurementStock
	var dishes []model.Dish
	var now time.Time
	err = s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			now = s.now().UTC()
			var err error
			stock, err = s.repo.Load(ctx, tx, filter, now)
			if err != nil {
				return err
			}
			active := true
			dishes, err = s.menu.Load(ctx, tx, &model.DishFilter{IsActive: &active}, now)
			return err
		},
	)
	if err != nil {
		return dto.ProcurementListResponse{}, err
	}
	items, err := BuildRecommendations(stock, dishes)
	if err != nil {
		return dto.ProcurementListResponse{}, err
	}
	total := int64(len(items))
	page := []dto.ProcurementItem{}
	if offset < total {
		end := offset + int64(filter.PageSize)
		if end < offset || end > total {
			end = total
		}
		page = items[offset:end]
	}
	return dto.ProcurementListResponse{
		Items: page, Total: total, Page: filter.Page, PageSize: filter.PageSize,
		Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)},
	}, nil
}

func sortRecommendations(items []dto.ProcurementItem) {
	slices.SortFunc(items, func(a, b dto.ProcurementItem) int {
		if order := cmp.Compare(len(b.AffectedDishes), len(a.AffectedDishes)); order != 0 {
			return order
		}
		aZero, bZero := a.AvailableQuantity == "0.000", b.AvailableQuantity == "0.000"
		if aZero != bZero {
			if aZero {
				return -1
			}
			return 1
		}
		if order := cmp.Compare(a.ProductName, b.ProductName); order != 0 {
			return order
		}
		return slices.Compare(a.ProductID[:], b.ProductID[:])
	})
}

func recommendations(stock []model.ProcurementStock) ([]dto.ProcurementItem, error) {
	items := make([]dto.ProcurementItem, 0, len(stock))
	for i := range stock {
		row := &stock[i]
		p := row.Product
		if p.TargetStock == nil {
			continue
		}
		target, err := quantity.ParseMilli(*p.TargetStock)
		if err != nil {
			return nil, fmt.Errorf("invalid procurement target for %s: %w", p.ID, err)
		}
		available, err := quantity.ParseMilli(row.AvailableQuantity)
		if err != nil {
			return nil, err
		}
		expired, err := quantity.ParseMilli(row.ExpiredQuantity)
		if err != nil {
			return nil, err
		}
		if target.Cmp(available) <= 0 {
			continue
		}
		codes := []string{"below_target"}
		reason := "Годный остаток ниже целевого"
		if available.Sign() == 0 {
			codes = append(codes, "no_valid_stock")
			reason = "Годного остатка нет"
			if expired.Sign() > 0 {
				codes = append(codes, "only_expired_stock")
				reason = "Остались только просроченные партии"
			}
		}
		unit := map[string]string{"g": "г", "ml": "мл", "pcs": "шт"}[p.Unit]
		if unit == "" {
			unit = p.Unit
		}
		formattedTarget := strings.TrimRight(strings.TrimRight(quantity.FormatMilli(target), "0"), ".")
		items = append(items, dto.ProcurementItem{
			ProductID:         p.ID,
			ProductName:       p.Name,
			Unit:              p.Unit,
			DisplayUnit:       p.DisplayUnit,
			TargetStock:       quantity.FormatMilli(target),
			AvailableQuantity: quantity.FormatMilli(available),
			ExpiredQuantity: quantity.FormatMilli(
				expired,
			),
			RecommendedQuantity: quantity.FormatMilli(new(big.Int).Sub(target, available)),
			ReasonCodes:         codes,
			Reason:              fmt.Sprintf("%s; пополнить до целевого запаса %s %s", reason, formattedTarget, unit),
			AffectedDishes:      []dto.AffectedDish{},
		})
	}
	return items, nil
}

func affectedDishes(dishes []model.Dish) (map[uuid.UUID][]dto.AffectedDish, error) {
	linked := make(map[uuid.UUID][]dto.AffectedDish)
	for _, dish := range dishes {
		availability, err := menu.CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return nil, fmt.Errorf("calculate procurement availability for %s: %w", dish.ID, err)
		}
		for i := range availability.Shortages {
			shortage := &availability.Shortages[i]
			linked[shortage.ProductID] = append(
				linked[shortage.ProductID],
				dto.AffectedDish{ID: dish.ID, Name: dish.Name},
			)
		}
	}
	for productID, dishes := range linked {
		slices.SortFunc(dishes, func(a, b dto.AffectedDish) int {
			if order := cmp.Compare(a.Name, b.Name); order != 0 {
				return order
			}
			return slices.Compare(a.ID[:], b.ID[:])
		})
		linked[productID] = slices.CompactFunc(dishes, func(a, b dto.AffectedDish) bool { return a.ID == b.ID })
	}
	return linked, nil
}

func BuildRecommendations(stock []model.ProcurementStock, dishes []model.Dish) ([]dto.ProcurementItem, error) {
	items, err := recommendations(stock)
	if err != nil {
		return nil, err
	}
	affected, err := affectedDishes(dishes)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if linked := affected[items[i].ProductID]; linked != nil {
			items[i].AffectedDishes = linked
		}
	}
	sortRecommendations(items)
	return items, nil
}
