package hotoffers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type transactionsStub struct {
	t     *testing.T
	calls int
	err   error
}

func (s *transactionsStub) WithinTx(ctx context.Context, opts *pgx.TxOptions, fn func(repository.DBTX) error) error {
	s.calls++
	if opts.IsoLevel != pgx.RepeatableRead || opts.AccessMode != pgx.ReadOnly {
		s.t.Fatal("hot offers need a consistent read-only snapshot")
	}
	if err := fn(nil); err != nil {
		return err
	}
	return s.err
}

type menuStub struct {
	t      *testing.T
	dishes []model.Dish
	now    time.Time
	search *string
	err    error
}

func (s *menuStub) Load(
	_ context.Context,
	_ repository.DBTX,
	filter *model.DishFilter,
	now time.Time,
) ([]model.Dish, error) {
	if filter.IsActive == nil || !*filter.IsActive || filter.Search != s.search || filter.Page != 0 ||
		!now.Equal(s.now) ||
		now.Location() != time.UTC {
		s.t.Fatal("menu filter or timestamp differs")
	}
	return s.dishes, s.err
}

type batchesStub struct {
	t        *testing.T
	products []uuid.UUID
	batches  []model.OrderBatch
	now      time.Time
	hours    int
	err      error
}

func (s *batchesStub) Load(
	_ context.Context,
	_ repository.DBTX,
	products []uuid.UUID,
	now time.Time,
) ([]model.OrderBatch, int, error) {
	if !reflect.DeepEqual(products, s.products) || !now.Equal(s.now) || now.Location() != time.UTC {
		s.t.Fatal("batch products or timestamp differs")
	}
	return s.batches, s.hours, s.err
}

func TestListRanksAndErrorHandling(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.FixedZone("MSK", 10800))
	product := uuid.New()
	a, b := uuid.New(), uuid.New()
	dishes := []model.Dish{testDish(a, product, "120.000", "1800.000"), testDish(b, product, "240.000", "1800.000")}
	search := "Пицца"
	menu := &menuStub{t: t, dishes: dishes, now: now, search: &search}
	batches := &batchesStub{
		t:        t,
		products: []uuid.UUID{product},
		now:      now,
		hours:    24,
		batches: []model.OrderBatch{
			{
				ID:                uuid.New(),
				ProductID:         product,
				SupplyID:          uuid.New(),
				SupplyNumber:      "104",
				LineNumber:        1,
				RemainingQuantity: "1800.000",
				ExpiresAt:         now.Add(18 * time.Hour),
			},
		},
	}
	tx := &transactionsStub{t: t}
	s := NewService(batches, menu, tx)
	clockCalls := 0
	s.now = func() time.Time { clockCalls++; return now }
	result, err := s.List(context.Background(), model.HotOffersFilter{Page: 2, PageSize: 1, Search: &search})
	if err != nil || result.Total != 2 || len(result.Items) != 1 || clockCalls != 1 {
		t.Fatalf("list: %+v %v, clock calls %d", result, err, clockCalls)
	}
	item := result.Items[0]
	if item.Dish.ID != a || item.Dish.RecipeVersion != 1 || item.Rank != 2 || item.PriorityScore != "1.6667" ||
		item.PriorityLevel != "low" ||
		item.AvailablePortions != "15" ||
		result.Meta["server_time"] != now.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("offer: %+v", result)
	}
	p := item.ProblemBatches[0]
	if p.RemainingQuantity != "1800.000" || p.UsedForOnePortion != "120.000" || p.RemainingSeconds != 64800 ||
		p.ExpiresAt.Location() != time.UTC ||
		!strings.Contains(item.Reason, "120 г") ||
		!strings.Contains(item.Reason, "18 ч") ||
		!strings.Contains(item.Reason, "№104") {
		t.Fatalf("batch facts: %+v reason %q", p, item.Reason)
	}
	result, err = s.List(context.Background(), model.HotOffersFilter{Page: 3, PageSize: 1, Search: &search})
	if err != nil || result.Total != 2 || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("empty page: %+v %v", result, err)
	}
	calls := tx.calls
	if _, err := s.List(
		context.Background(),
		model.HotOffersFilter{Page: 0, PageSize: 1},
	); err == nil ||
		tx.calls != calls {
		t.Fatal("invalid pagination opened transaction")
	}
	for _, stage := range []string{"menu", "batches", "commit", "quantity"} {
		t.Run(stage, func(t *testing.T) {
			want := errors.New(stage)
			switch stage {
			case "menu":
				menu.err = want
			case "batches":
				batches.err = want
			case "commit":
				tx.err = want
			case "quantity":
				batches.batches[0].RemainingQuantity = "broken"
			}
			result, err := s.List(context.Background(), model.HotOffersFilter{Page: 1, PageSize: 1, Search: &search})
			if err == nil || result.Items != nil || result.Meta != nil {
				t.Fatalf("partial success: %+v %v", result, err)
			}
			if stage != "quantity" && !errors.Is(err, want) {
				t.Fatal(err)
			}
			menu.err = nil
			batches.err = nil
			tx.err = nil
		})
	}
}

func TestExpiryText(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		want    string
	}{{0, "менее чем через секунду"}, {1, "через 1 с"}, {60, "через 1 мин"}, {3661, "через 1 ч 1 мин 1 с"}} {
		if got := expiryText(tc.seconds); got != tc.want {
			t.Fatalf("%d: %q", tc.seconds, got)
		}
	}
}

func (s *batchesStub) Preferences(context.Context, repository.DBTX) (map[uuid.UUID]model.HotOfferPreference, error) {
	return map[uuid.UUID]model.HotOfferPreference{}, nil
}
