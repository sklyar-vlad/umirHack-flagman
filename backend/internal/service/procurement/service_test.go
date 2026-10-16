package procurement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func TestRecommendationsExactTargets(t *testing.T) {
	for _, tc := range []struct {
		name, target, available, expired, want string
	}{
		{"unset", "", "0.000", "50.000", ""},
		{"zero", "0.000", "0.000", "50.000", ""},
		{"at target", "1.000", "1.000", "0.000", ""},
		{"above target", "1.000", "2.000", "0.000", ""},
		{"mixed stock", "1.000", "0.999", "50.000", "0.001"},
		{"expired only", "1.000", "0.000", "50.000", "1.000"},
		{"large aggregate", "99999999999.999", "199999999999.998", "0.000", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := model.Product{ID: uuid.New(), Name: tc.name, Unit: "g", DisplayUnit: "kg"}
			if tc.target != "" {
				p.TargetStock = &tc.target
			}
			items, err := recommendations(
				[]model.ProcurementStock{{Product: p, AvailableQuantity: tc.available, ExpiredQuantity: tc.expired}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(items) != 0 {
					t.Fatalf("unexpected recommendation: %+v", items)
				}
				return
			}
			if len(items) != 1 || items[0].RecommendedQuantity != tc.want || items[0].TargetStock != tc.target ||
				items[0].ProductID != p.ID ||
				items[0].AffectedDishes == nil {
				t.Fatalf("recommendation: %+v", items)
			}
		})
	}
}

func TestRecommendationsRejectCorruptQuantities(t *testing.T) {
	for _, values := range [][3]string{{"-1", "0", "0"}, {"1", "broken", "0"}, {"1", "0", "0.0001"}} {
		_, err := recommendations(
			[]model.ProcurementStock{
				{
					Product:           model.Product{TargetStock: &values[0]},
					AvailableQuantity: values[1],
					ExpiredQuantity:   values[2],
				},
			},
		)
		if err == nil {
			t.Fatalf("accepted corrupt quantities: %v", values)
		}
	}
}

type transactionStub struct {
	t     *testing.T
	err   error
	calls int
}

func (tx *transactionStub) WithinTx(ctx context.Context, opts *pgx.TxOptions, fn func(repository.DBTX) error) error {
	tx.calls++
	if opts.IsoLevel != pgx.RepeatableRead || opts.AccessMode != pgx.ReadOnly {
		tx.t.Fatal("procurement must use a read-only repeatable-read snapshot")
	}
	if err := fn(nil); err != nil {
		return err
	}
	return tx.err
}

type stockStub struct {
	items []model.ProcurementStock
	err   error
}

func (r stockStub) Load(
	context.Context,
	repository.DBTX,
	model.ProcurementFilter,
	time.Time,
) ([]model.ProcurementStock, error) {
	return r.items, r.err
}

func TestListPaginationAndErrors(t *testing.T) {
	target := "1.000"
	r := stockStub{items: []model.ProcurementStock{
		{Product: model.Product{Name: "A", TargetStock: &target}, AvailableQuantity: "0.500", ExpiredQuantity: "0"},
		{Product: model.Product{Name: "B", TargetStock: &target}, AvailableQuantity: "0", ExpiredQuantity: "0"},
	}}
	tx := &transactionStub{t: t}
	s := NewService(r, menuStub{}, tx)
	s.now = func() time.Time { return time.Date(2026, 10, 11, 3, 0, 0, 0, time.FixedZone("MSK", 3*3600)) }
	result, err := s.List(context.Background(), model.ProcurementFilter{Page: 3, PageSize: 1})
	if err != nil || result.Total != 2 || result.Items == nil || len(result.Items) != 0 ||
		result.Meta["server_time"] != "2026-10-11T00:00:00Z" {
		t.Fatalf("empty page: %+v %v", result, err)
	}
	if _, err := s.List(
		context.Background(),
		model.ProcurementFilter{Page: 0, PageSize: 1},
	); err == nil ||
		tx.calls != 1 {
		t.Fatalf("invalid page reached transaction: %v", err)
	}
	failure := errors.New("read failure")
	for _, service := range []*Service{NewService(stockStub{err: failure}, menuStub{}, tx), NewService(r, menuStub{err: failure}, tx), NewService(r, menuStub{}, &transactionStub{t: t, err: failure})} {
		result, err := service.List(context.Background(), model.ProcurementFilter{Page: 1, PageSize: 50})
		if !errors.Is(err, failure) || result.Items != nil {
			t.Fatalf("partial result on failure: %+v %v", result, err)
		}
	}
}

type menuStub struct {
	dishes []model.Dish
	err    error
}

func (r menuStub) Load(context.Context, repository.DBTX, *model.DishFilter, time.Time) ([]model.Dish, error) {
	return r.dishes, r.err
}

func TestAffectedDishesRejectCorruptRecipe(t *testing.T) {
	_, err := affectedDishes(
		[]model.Dish{
			{
				ID:       uuid.New(),
				IsActive: true,
				Recipe: []model.RecipeIngredient{
					{QuantityPerPortion: "0.000", AvailableQuantity: "1.000", ExpiredQuantity: "0.000"},
				},
			},
		},
	)
	if err == nil {
		t.Fatal("invalid recipe produced an explanation")
	}
}

func TestRecommendationPriority(t *testing.T) {
	lowID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	highID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	items := []dto.ProcurementItem{
		{ProductName: "same", ProductID: highID, AvailableQuantity: "0.000"},
		{ProductName: "AAA", AvailableQuantity: "1.000"},
		{ProductName: "same", ProductID: lowID, AvailableQuantity: "0.000"},
		{ProductName: "ZZZ", AvailableQuantity: "2.000", AffectedDishes: make([]dto.AffectedDish, 2)},
		{ProductName: "BBB", AvailableQuantity: "0.000", AffectedDishes: make([]dto.AffectedDish, 1)},
		{ProductName: "aaa", AvailableQuantity: "0.000"},
	}
	sortRecommendations(items)
	wantNames := []string{"ZZZ", "BBB", "aaa", "same", "same", "AAA"}
	for i, name := range wantNames {
		if items[i].ProductName != name {
			t.Fatalf("priority: %+v", items)
		}
	}
	if items[3].ProductID != lowID || items[4].ProductID != highID {
		t.Fatalf("unstable ID tie: %+v", items)
	}
}
