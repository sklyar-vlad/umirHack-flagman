package dashboard

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type repositoryStub struct {
	stage     string
	err       error
	data      model.DashboardData
	dishes    []model.Dish
	batches   []model.OrderBatch
	timestamp time.Time
}

func (r *repositoryStub) Load(_ context.Context, _ repository.DBTX, now time.Time) (model.DashboardData, error) {
	r.timestamp = now
	if r.stage == "stock" {
		return model.DashboardData{}, r.err
	}
	return r.data, nil
}

func (r *repositoryStub) LoadRecipes(context.Context, repository.DBTX) ([]model.Dish, error) {
	if r.stage == "recipes" {
		return nil, r.err
	}
	return r.dishes, nil
}

func (r *repositoryStub) LoadBatches(context.Context, repository.DBTX) ([]model.OrderBatch, error) {
	if r.stage == "batches" {
		return nil, r.err
	}
	return r.batches, nil
}

type transactionsStub struct {
	t     *testing.T
	err   error
	calls int
}

func (s *transactionsStub) WithinTx(_ context.Context, opts *pgx.TxOptions, fn func(repository.DBTX) error) error {
	s.calls++
	if opts.IsoLevel != pgx.RepeatableRead || opts.AccessMode != pgx.ReadOnly {
		s.t.Fatal("invalid snapshot options")
	}
	if err := fn(nil); err != nil {
		return err
	}
	return s.err
}

func TestDashboardErrorsNeverReturnPartialResponse(t *testing.T) {
	for _, stage := range []string{"stock", "recipes", "batches", "commit", "quantity", "missing_stock", "availability", "offers", "procurement"} {
		t.Run(stage, func(t *testing.T) {
			want := errors.New(stage)
			product := uuid.New()
			version := 1
			now := time.Now().UTC()
			r := &repositoryStub{stage: stage, err: want, data: model.DashboardData{
				Settings: model.DashboardSettings{ExpiringThresholdHours: 24},
				Stock:    []model.ProcurementStock{{Product: model.Product{ID: product, Unit: "g"}, AvailableQuantity: "1.000", ExpiredQuantity: "0.000"}},
			}, dishes: []model.Dish{{ID: uuid.New(), IsActive: true, RecipeVersion: &version, Recipe: []model.RecipeIngredient{{ProductID: product, Unit: "g", QuantityPerPortion: "1.000"}}}},
				batches: []model.OrderBatch{{ID: uuid.New(), ProductID: product, RemainingQuantity: "1.000", ExpiresAt: now.Add(time.Hour)}}}
			tx := &transactionsStub{t: t}
			switch stage {
			case "commit":
				tx.err = want
			case "quantity":
				r.data.Stock[0].AvailableQuantity = "broken"
			case "missing_stock":
				r.data.Stock = nil
			case "availability":
				r.dishes[0].Recipe[0].QuantityPerPortion = "broken"
			case "offers":
				r.batches[0].RemainingQuantity = "broken"
			case "procurement":
				target := "broken"
				r.data.Stock[0].Product.TargetStock = &target
			}
			s := NewService(r, tx)
			s.now = func() time.Time { return now }
			result, err := s.Get(context.Background(), "manager")
			if err == nil || !reflect.DeepEqual(result, dto.DashboardResponse{}) {
				t.Fatalf("partial result: %+v %v", result, err)
			}
			if (stage == "stock" || stage == "recipes" || stage == "batches" || stage == "commit") && !errors.Is(err, want) {
				t.Fatal(err)
			}
		})
	}
}

func TestDashboardClockUTCAndServiceRoleGuard(t *testing.T) {
	r := &repositoryStub{data: model.DashboardData{Settings: model.DashboardSettings{ExpiringThresholdHours: 24}}}
	tx := &transactionsStub{t: t}
	s := NewService(r, tx)
	now := time.Date(2026, 10, 11, 9, 0, 0, 1, time.FixedZone("MSK", 10800))
	clockCalls := 0
	s.now = func() time.Time { clockCalls++; return now }
	result, err := s.Get(context.Background(), "order_operator")
	if err != nil || clockCalls != 1 || tx.calls != 1 || r.timestamp.Location() != time.UTC ||
		!r.timestamp.Equal(now) || result.Meta["server_time"] != now.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("clock: %+v %v %d", result, err, clockCalls)
	}
	if _, err := s.Get(context.Background(), "unknown"); !errors.Is(err, apperrors.ErrForbidden) || tx.calls != 1 {
		t.Fatal("unknown role opened transaction")
	}
}
