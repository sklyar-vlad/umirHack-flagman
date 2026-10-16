package hotoffers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	menuservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const (
	roleAdmin   = "admin"
	roleManager = "manager"
)

type Repository interface {
	Preferences(context.Context, repository.DBTX) (map[uuid.UUID]model.HotOfferPreference, error)
	Load(context.Context, repository.DBTX, []uuid.UUID, time.Time) ([]model.OrderBatch, int, error)
}

type MenuRepository interface {
	Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	menu         MenuRepository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, menu MenuRepository, transactions Transactions) *Service {
	return &Service{repo: repo, menu: menu, transactions: transactions, now: time.Now}
}

func (s *Service) List(ctx context.Context, filter model.HotOffersFilter) (dto.HotOffersListResponse, error) {
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	var preferences map[uuid.UUID]model.HotOfferPreference
	var dishes []model.Dish
	var batches []model.OrderBatch
	var hours int
	var now time.Time
	err = s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			now = s.now().UTC()
			active := true
			var err error
			dishes, err = s.menu.Load(ctx, tx, &model.DishFilter{IsActive: &active, Search: filter.Search}, now)
			if err != nil {
				return err
			}
			batches, hours, err = s.repo.Load(ctx, tx, ingredientProducts(dishes), now)
			if err != nil {
				return err
			}
			preferences, err = s.repo.Preferences(ctx, tx)
			return err
		},
	)
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	items, err := BuildOffers(dishes, batches, now, hours)
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	actor, _ := requestctx.Actor(ctx)
	items, err = ApplyPreferences(items, dishes, preferences, actor.Role)
	if err != nil {
		return dto.HotOffersListResponse{}, err
	}
	total := int64(len(items))
	page := []dto.HotOffer{}
	if offset < total {
		end := offset + int64(filter.PageSize)
		if end < offset || end > total {
			end = total
		}
		page = items[offset:end]
	}
	return dto.HotOffersListResponse{
		Items:    page,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Meta:     map[string]any{"server_time": now.Format(time.RFC3339Nano)},
	}, nil
}

func ingredientProducts(dishes []model.Dish) []uuid.UUID {
	products := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, dish := range dishes {
		for _, ingredient := range dish.Recipe {
			if !seen[ingredient.ProductID] {
				products = append(products, ingredient.ProductID)
				seen[ingredient.ProductID] = true
			}
		}
	}
	return products
}

// BuildOffers independently simulates each dish and returns the full ranked list
// from the caller's snapshot and timestamp, without reads or stock mutation.
func BuildOffers(dishes []model.Dish, batches []model.OrderBatch, now time.Time, hours int) ([]dto.HotOffer, error) {
	offers, err := simulate(dishes, batches, now, hours)
	if err != nil {
		return nil, err
	}
	scored, err := scoreOffers(offers, now, hours)
	if err != nil {
		return nil, err
	}
	items := make([]dto.HotOffer, 0, len(scored))
	for i := range scored {
		items = append(items, renderOffer(&scored[i], int64(i+1), now))
	}
	return items, nil
}

// ApplyPreferences is shared by the list and dashboard to enforce publication everywhere.
func ApplyPreferences(
	items []dto.HotOffer,
	dishes []model.Dish,
	preferences map[uuid.UUID]model.HotOfferPreference,
	role string,
) ([]dto.HotOffer, error) {
	seen := map[uuid.UUID]bool{}
	for i := range items {
		item := items[i]
		seen[item.Dish.ID] = true
	}
	for _, dish := range dishes {
		pref, ok := preferences[dish.ID]
		if !ok || !pref.Manual || seen[dish.ID] || dish.RecipeVersion == nil {
			continue
		}
		availability, err := menuservice.CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return nil, err
		}
		if !availability.IsAvailable {
			continue
		}
		items = append(
			items,
			dto.HotOffer{
				Dish:              dto.HotOfferDish{ID: dish.ID, Name: dish.Name, RecipeVersion: *dish.RecipeVersion},
				Published:         true,
				Manual:            true,
				PriorityScore:     "0.0000",
				PriorityLevel:     "low",
				AvailablePortions: availability.AvailablePortions,
				Reason:            "Выбор менеджера",
				ProblemBatches:    []dto.HotOfferBatch{},
			},
		)
	}
	management := role == roleAdmin || role == roleManager
	visible := make([]dto.HotOffer, 0, len(items))
	for i := range items {
		item := items[i]
		if pref, ok := preferences[item.Dish.ID]; ok {
			item.Published = pref.Published
			item.Manual = pref.Manual
			item.Message = pref.Message
			item.PreferenceRevision = pref.Revision
		}
		if !item.Published && !management {
			continue
		}
		item.Rank = int64(len(visible) + 1)
		visible = append(visible, item)
	}
	return visible, nil
}
