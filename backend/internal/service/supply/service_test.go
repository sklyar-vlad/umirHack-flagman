package supply

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type fakeRepository struct {
	Repository
	products map[uuid.UUID]model.SupplyProduct
	exists   bool
	ids      []uuid.UUID
}

func (r *fakeRepository) FindProducts(
	_ context.Context, _ repository.DBTX, ids []uuid.UUID,
) (map[uuid.UUID]model.SupplyProduct, error) {
	r.ids = ids
	return r.products, nil
}

func (r *fakeRepository) NumberExists(
	_ context.Context, _ repository.DBTX, _ string,
) (bool, error) {
	return r.exists, nil
}

func TestValidateBasics(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	expiresAt := now.Add(24 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name     string
		unit     string
		quantity string
		number   string
		inactive bool
		missing  bool
		future   bool
		conflict bool
		field    string
	}{
		{name: "whole pcs", unit: "pcs", quantity: "2.000", number: "  104  "},
		{name: "fractional g", unit: "g", quantity: "0.001", number: "104"},
		{name: "fractional ml", unit: "ml", quantity: "12.375", number: "104"},
		{name: "max quantity", unit: "g", quantity: "99999999999.999", number: "104"},
		{name: "80 Unicode characters", unit: "g", quantity: "1", number: strings.Repeat("я", 80)},
		{name: "zero", unit: "g", quantity: "0.000", number: "104", field: "items[0].quantity"},
		{name: "negative", unit: "g", quantity: "-1", number: "104", field: "items[0].quantity"},
		{name: "exponent", unit: "g", quantity: "1e3", number: "104", field: "items[0].quantity"},
		{name: "precision", unit: "g", quantity: "1.0001", number: "104", field: "items[0].quantity"},
		{name: "overflow", unit: "g", quantity: "100000000000", number: "104", field: "items[0].quantity"},
		{name: "fractional pcs", unit: "pcs", quantity: "1.5", number: "104", field: "items[0].quantity"},
		{name: "empty number", unit: "g", quantity: "1", number: "  ", field: "number"},
		{name: "long number", unit: "g", quantity: "1", number: strings.Repeat("я", 81), field: "number"},
		{name: "NUL", unit: "g", quantity: "1", number: "104\x00", field: "number"},
		{name: "inactive", unit: "g", quantity: "1", number: "104", inactive: true, field: "items[0].product_id"},
		{name: "missing", unit: "g", quantity: "1", number: "104", missing: true, field: "items[0].product_id"},
		{name: "future", unit: "g", quantity: "1", number: "104", future: true, field: "received_at"},
		{name: "duplicate number", unit: "g", quantity: "1", number: "104", conflict: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{
				products: map[uuid.UUID]model.SupplyProduct{
					id: {ID: id, Unit: tt.unit, IsActive: !tt.inactive},
				},
				exists: tt.conflict,
			}
			if tt.missing {
				delete(repo.products, id)
			}
			s := NewService(repo, nil, nil)
			s.now = func() time.Time { return now }
			receivedAt := now
			if tt.future {
				receivedAt = now.Add(time.Second)
			}
			input := dto.CreateSupplyRequest{
				Number: tt.number, ReceivedAt: receivedAt.Format(time.RFC3339Nano),
				Items: []dto.CreateSupplyItem{{ProductID: id.String(), Quantity: tt.quantity, ExpiresAt: &expiresAt}},
			}
			result, err := s.ValidateBasics(context.Background(), nil, input)
			if tt.field != "" {
				var fieldErr *validations.FieldError
				if !errors.As(err, &fieldErr) || fieldErr.Field != tt.field {
					t.Fatalf("want field %s, got %v", tt.field, err)
				}
				return
			}
			if tt.conflict {
				if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("want conflict, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Number != strings.TrimSpace(tt.number) ||
				result.Items[0].Unit != tt.unit || result.Items[0].QuantityMilli <= 0 {
				t.Fatalf("unexpected normalized result: %+v", result)
			}
		})
	}
}

func TestValidateBasicsItemsAndDate(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	expiresAt := now.Add(24 * time.Hour).Format(time.RFC3339)
	repo := &fakeRepository{products: map[uuid.UUID]model.SupplyProduct{
		id: {ID: id, Unit: "pcs", IsActive: true},
	}}
	s := NewService(repo, nil, nil)
	s.now = func() time.Time { return now }

	input := dto.CreateSupplyRequest{
		Number: "105", ReceivedAt: "2026-10-10T15:00:00+03:00",
		Items: []dto.CreateSupplyItem{{ProductID: id.String(), Quantity: "1", ExpiresAt: &expiresAt}},
	}
	input.Items = append(input.Items, input.Items[0])
	result, err := s.ValidateBasics(context.Background(), nil, input)
	if err != nil || len(result.Items) != 2 || len(repo.ids) != 1 ||
		!result.ReceivedAt.Equal(now) || result.ReceivedAt.Location() != time.UTC {
		t.Fatalf("repeated product/UTC: result=%+v, err=%v", result, err)
	}

	tests := []struct {
		name   string
		change func(*dto.CreateSupplyRequest)
		field  string
	}{
		{"empty items", func(r *dto.CreateSupplyRequest) { r.Items = nil }, "items"},
		{"too many", func(r *dto.CreateSupplyRequest) {
			r.Items = make([]dto.CreateSupplyItem, 101)
		}, "items"},
		{"missing timezone", func(r *dto.CreateSupplyRequest) {
			r.ReceivedAt = "2026-10-10T12:00:00"
		}, "received_at"},
		{"invalid UUID", func(r *dto.CreateSupplyRequest) {
			r.Items[0].ProductID = "bad"
		}, "items[0].product_id"},
		{"zero UUID", func(r *dto.CreateSupplyRequest) {
			r.Items[0].ProductID = uuid.Nil.String()
		}, "items[0].product_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := dto.CreateSupplyRequest{
				Number: "105", ReceivedAt: now.Format(time.RFC3339),
				Items: []dto.CreateSupplyItem{{ProductID: id.String(), Quantity: "1", ExpiresAt: &expiresAt}},
			}
			tt.change(&req)
			_, err := s.ValidateBasics(context.Background(), nil, req)
			var fieldErr *validations.FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != tt.field {
				t.Fatalf("want field %s, got %v", tt.field, err)
			}
		})
	}
}
