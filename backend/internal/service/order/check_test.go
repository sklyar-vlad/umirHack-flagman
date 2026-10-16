package order

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type checkRepository struct {
	dishes []model.Dish
	err    error
	calls  int
	filter model.DishFilter
	now    time.Time
	ctx    context.Context
	tx     repository.DBTX
}

func (r *checkRepository) Load(
	ctx context.Context,
	tx repository.DBTX,
	filter *model.DishFilter,
	now time.Time,
) ([]model.Dish, error) {
	r.calls++
	r.filter, r.now, r.ctx, r.tx = *filter, now, ctx, tx
	return r.dishes, r.err
}

type checkTransactions struct {
	calls   int
	options pgx.TxOptions
	err     error
	tx      repository.DBTX
}

func (r *checkTransactions) WithinTx(_ context.Context, options *pgx.TxOptions, fn func(repository.DBTX) error) error {
	r.calls++
	r.options = *options
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.err
}

func TestCheckAggregatesSharedIngredients(t *testing.T) {
	product := uuid.New()
	first, second := uuid.New(), uuid.New()
	version := 1
	for _, tc := range []struct {
		name, stock, expired, required, missing, reason string
		firstQuantity, secondQuantity                   string
		portions                                        int
	}{
		{"shared deficit", "1.000", "0.000", "1.200", "0.200", "ingredient_shortage", "0.300", "0.300", 2},
		{"exact stock", "1.200", "0.000", "1.200", "0.000", "", "0.300", "0.300", 2},
		{"surplus", "2.000", "9.000", "1.200", "0.000", "", "0.300", "0.300", 2},
		{"only expired", "0.000", "10.000", "1.200", "1.200", "only_expired_stock", "0.300", "0.300", 2},
		{"mixed stock", "0.500", "10.000", "1.200", "0.700", "ingredient_shortage", "0.300", "0.300", 2},
		{"empty stock", "0.000", "0.000", "1.200", "1.200", "ingredient_shortage", "0.300", "0.300", 2},
		{"thousandths", "0.007", "0.000", "0.007", "0.000", "", "0.001", "0.002", 2},
		{"large aggregate", "199999999999998.000", "0.000", "199999999999998.000", "0.000", "", "99999999999.999", "99999999999.999", 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstPortions := tc.portions
			if tc.name == "thousandths" {
				firstPortions = 3
			}
			r := &checkRepository{dishes: []model.Dish{
				{ID: second, Name: "A", IsActive: true, RecipeVersion: &version, Recipe: []model.RecipeIngredient{
					{
						ProductID:          product,
						ProductName:        "Cheese",
						Unit:               "g",
						QuantityPerPortion: tc.secondQuantity,
						AvailableQuantity:  tc.stock,
						ExpiredQuantity:    tc.expired,
					},
				}},
				{ID: first, Name: "Z", IsActive: true, RecipeVersion: &version, Recipe: []model.RecipeIngredient{
					{
						ProductID:          product,
						ProductName:        "Cheese",
						Unit:               "g",
						QuantityPerPortion: tc.firstQuantity,
						AvailableQuantity:  tc.stock,
						ExpiredQuantity:    tc.expired,
					},
				}},
			}}
			s := NewService(r, &checkTransactions{})
			response, err := s.Check(context.Background(), dto.OrderRequest{Items: []dto.OrderRequestItem{
				{DishID: first.String(), Portions: firstPortions}, {DishID: second.String(), Portions: tc.portions},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Requirements) != 1 || response.Requirements[0].RequiredQuantity != tc.required ||
				response.Requirements[0].AvailableQuantity != tc.stock ||
				response.Requirements[0].MissingQuantity != tc.missing {
				t.Fatalf("requirements: %+v", response.Requirements)
			}
			if response.CanCreate != (tc.reason == "") || len(response.Issues) != 0 || response.Shortages == nil {
				t.Fatalf("unexpected result: %+v", response)
			}
			if tc.reason != "" &&
				(len(response.Shortages) != 1 || response.Shortages[0].ReasonCode != tc.reason || response.Shortages[0].ExpiredQuantity != tc.expired) {
				t.Fatalf("shortages: %+v", response.Shortages)
			}
			if response.Items[0].DishID != first || response.Items[1].DishID != second {
				t.Fatalf("basket order changed: %+v", response.Items)
			}
		})
	}
}

func TestCheckSnapshotAndValidation(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	version := 1
	r := &checkRepository{dishes: []model.Dish{{ID: first, IsActive: true, RecipeVersion: &version}}}
	transactions := &checkTransactions{}
	s := NewService(r, transactions)
	now := time.Date(2026, 10, 10, 20, 0, 0, 123, time.FixedZone("MSK", 3*3600))
	clockCalls := 0
	s.now = func() time.Time { clockCalls++; return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.Check(ctx, dto.OrderRequest{})
	if err == nil || transactions.calls != 0 || r.calls != 0 || clockCalls != 0 {
		t.Fatalf("invalid basket read the database: %v", err)
	}
	response, err := s.Check(
		ctx,
		dto.OrderRequest{
			Items: []dto.OrderRequestItem{{DishID: first.String(), Portions: 2}, {DishID: first.String(), Portions: 3}},
		},
	)
	if err != nil || response.Items[0].Portions != 5 {
		t.Fatalf("merged basket: %+v %v", response, err)
	}
	if transactions.calls != 1 || r.calls != 1 || clockCalls != 1 || r.ctx != ctx || r.now.Location() != time.UTC ||
		!r.now.Equal(now) ||
		!reflect.DeepEqual(r.filter.IDs, []uuid.UUID{first}) {
		t.Fatalf("inconsistent snapshot: %+v %+v", r, transactions)
	}
	if transactions.options.IsoLevel != pgx.RepeatableRead || transactions.options.AccessMode != pgx.ReadOnly ||
		response.Meta["server_time"] != now.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("snapshot options/time: %+v %+v", transactions.options, response.Meta)
	}
	_, err = s.Check(
		ctx,
		dto.OrderRequest{
			Items: []dto.OrderRequestItem{
				{DishID: first.String(), Portions: 1},
				{DishID: first.String(), Portions: 1},
				{DishID: second.String(), Portions: 1},
			},
		},
	)
	var fieldError *validations.FieldError
	if !errors.As(err, &fieldError) || fieldError.Field != "items[2].dish_id" {
		t.Fatalf("unknown dish must reference original input index: %v", err)
	}
}

func TestCheckPropagatesFailures(t *testing.T) {
	id, product := uuid.New(), uuid.New()
	version := 1
	sentinel := errors.New("database failed")
	for _, stage := range []string{"load", "commit", "recipe", "stock", "expired stock"} {
		t.Run(stage, func(t *testing.T) {
			ingredient := model.RecipeIngredient{
				ProductID:          product,
				QuantityPerPortion: "0.001",
				AvailableQuantity:  "1.000",
				ExpiredQuantity:    "0.000",
			}
			r := &checkRepository{}
			transactions := &checkTransactions{}
			switch stage {
			case "load":
				r.err = sentinel
			case "commit":
				transactions.err = sentinel
			case "recipe":
				ingredient.QuantityPerPortion = "0.000"
			case "stock":
				ingredient.AvailableQuantity = "-1.000"
			case "expired stock":
				ingredient.ExpiredQuantity = "bad"
			}
			r.dishes = []model.Dish{
				{ID: id, IsActive: true, RecipeVersion: &version, Recipe: []model.RecipeIngredient{ingredient}},
			}
			response, err := NewService(
				r,
				transactions,
			).Check(context.Background(), dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: id.String(), Portions: 1}}})
			if err == nil || response.Items != nil || response.CanCreate {
				t.Fatalf("failure produced a successful/partial result: %+v %v", response, err)
			}
			if (stage == "load" || stage == "commit") && !errors.Is(err, sentinel) {
				t.Fatalf("lost original failure: %v", err)
			}
		})
	}
}

func TestCheckRequirementsHaveStableOrder(t *testing.T) {
	id := uuid.New()
	version := 1
	low := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	high := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	r := &checkRepository{
		dishes: []model.Dish{{ID: id, IsActive: true, RecipeVersion: &version, Recipe: []model.RecipeIngredient{
			{
				ProductID:          high,
				ProductName:        "Same",
				Unit:               "pcs",
				QuantityPerPortion: "1.000",
				AvailableQuantity:  "0.000",
				ExpiredQuantity:    "0.000",
			},
			{
				ProductID:          low,
				ProductName:        "Same",
				Unit:               "g",
				QuantityPerPortion: "1.000",
				AvailableQuantity:  "0.000",
				ExpiredQuantity:    "0.000",
			},
		}}},
	}
	response, err := NewService(
		r,
		&checkTransactions{},
	).Check(context.Background(), dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: id.String(), Portions: 1}}})
	if err != nil || len(response.Requirements) != 2 || response.Requirements[0].ProductID != low ||
		response.Requirements[1].ProductID != high ||
		response.Shortages[0].ProductID != low ||
		response.Shortages[1].ProductID != high {
		t.Fatalf("unstable order: %+v %v", response, err)
	}
	if response.Requirements[0].Unit+","+response.Requirements[1].Unit != "g,pcs" {
		t.Fatal("different products/units were aggregated together")
	}
}

func TestCheckRecipeIssuesAndVersions(t *testing.T) {
	id, product := uuid.New(), uuid.New()
	one, two := 1, 2
	for _, tc := range []struct {
		name           string
		active, recipe bool
		current, sent  *int
		codes          []string
	}{
		{"current version", true, true, &two, &two, nil},
		{"version omitted", true, true, &two, nil, nil},
		{"stale version", true, true, &two, &one, []string{"recipe_changed"}},
		{"future version", true, true, &one, &two, []string{"recipe_changed"}},
		{"inactive", false, true, &two, &two, []string{"dish_inactive"}},
		{"no recipe", true, false, nil, nil, []string{"recipe_missing"}},
		{"empty latest recipe", true, false, &two, &two, []string{"recipe_missing"}},
		{"inactive and stale", false, true, &two, &one, []string{"dish_inactive", "recipe_changed"}},
		{"inactive and missing", false, false, nil, &one, []string{"dish_inactive", "recipe_missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dish := model.Dish{ID: id, Name: "Pizza", IsActive: tc.active, RecipeVersion: tc.current}
			if tc.recipe {
				dish.Recipe = []model.RecipeIngredient{
					{
						ProductID:          product,
						ProductName:        "Cheese",
						Unit:               "g",
						QuantityPerPortion: "0.300",
						AvailableQuantity:  "1.000",
						ExpiredQuantity:    "0.000",
					},
				}
			}
			response, err := NewService(
				&checkRepository{dishes: []model.Dish{dish}},
				&checkTransactions{},
			).Check(context.Background(), dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: id.String(), Portions: 2, RecipeVersion: tc.sent}}})
			if err != nil || response.CanCreate != (len(tc.codes) == 0) || len(response.Issues) != len(tc.codes) ||
				!reflect.DeepEqual(response.Items[0].RecipeVersion, tc.current) ||
				response.Shortages == nil ||
				response.Requirements == nil ||
				response.Issues == nil {
				t.Fatalf("issues/versions: %+v %v", response, err)
			}
			for i, issue := range response.Issues {
				if issue.Code != tc.codes[i] || issue.DishID != id || issue.Message == "" ||
					!reflect.DeepEqual(issue.CurrentRecipeVersion, tc.current) {
					t.Fatalf("wrong issue: %+v", issue)
				}
			}
			if tc.recipe && (len(response.Requirements) != 1 || response.Requirements[0].RequiredQuantity != "0.600") {
				t.Fatalf("requirements must use the current recipe even with issues: %+v", response.Requirements)
			}
		})
	}
}
