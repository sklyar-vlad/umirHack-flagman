package supply

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestValidateBasicsPreparesExpiry(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	received := now.Add(-time.Hour + 123456789*time.Nanosecond)
	produced := now.Add(-2*time.Hour + 987654321*time.Nanosecond)
	producedRaw := produced.Format(time.RFC3339Nano)
	expires := now.Add(24*time.Hour + 123456789*time.Nanosecond)
	expiresRaw := expires.Format(time.RFC3339Nano)
	id := uuid.New()

	tests := []struct {
		name     string
		item     dto.CreateSupplyItem
		standard *int
		source   model.ExpirySource
		hours    int
		field    string
		required bool
	}{
		{
			name:   "explicit",
			item:   dto.CreateSupplyItem{ExpiresAt: &expiresRaw},
			source: model.ExpiryExplicit,
		},
		{
			name:   "explicit with production",
			item:   dto.CreateSupplyItem{ExpiresAt: &expiresRaw, ProducedAt: &producedRaw},
			source: model.ExpiryExplicit,
		},
		{
			name:     "requested hours override standard",
			item:     dto.CreateSupplyItem{ProducedAt: &producedRaw, ShelfLifeHours: expiryPointer(48)},
			standard: expiryPointer(72),
			source:   model.ExpiryCalculated,
			hours:    48,
		},
		{
			name:     "standard hours are copied",
			item:     dto.CreateSupplyItem{ProducedAt: &producedRaw},
			standard: expiryPointer(72),
			source:   model.ExpiryCalculated,
			hours:    72,
		},
		{
			name:     "missing shelf life",
			item:     dto.CreateSupplyItem{ProducedAt: &producedRaw},
			field:    "items[0].shelf_life_hours",
			required: true,
		},
		{
			name:  "missing expiry input",
			field: "items[0].produced_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{products: map[uuid.UUID]model.SupplyProduct{
				id: {
					ID: id, Unit: "g", IsActive: true,
					StandardShelfLifeHours: tt.standard,
				},
			}}
			s := NewService(repo, nil, nil)
			s.now = func() time.Time { return now }
			item := tt.item
			item.ProductID = id.String()
			item.Quantity = "1"
			result, err := s.ValidateBasics(context.Background(), nil, dto.CreateSupplyRequest{
				Number: "108", ReceivedAt: received.Format(time.RFC3339Nano),
				Items: []dto.CreateSupplyItem{item},
			})
			if tt.field != "" {
				var field *validations.FieldError
				if !errors.As(err, &field) || field.Field != tt.field || len(result.Items) != 0 {
					t.Fatalf("expected rejection at %s, result=%+v err=%v", tt.field, result, err)
				}
				if tt.required && !errors.Is(err, apperrors.ErrShelfLifeRequired) {
					t.Fatalf("expected missing shelf life error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !result.ReceivedAt.Equal(received.Truncate(time.Microsecond)) {
				t.Fatalf("unexpected receipt precision: %s", result.ReceivedAt)
			}
			expiry := result.Items[0].Expiry
			if expiry.Source != tt.source {
				t.Fatalf("source=%s, want %s", expiry.Source, tt.source)
			}
			if item.ProducedAt == nil {
				if expiry.ProducedAt != nil {
					t.Fatal("explicit expiry must not invent a production date")
				}
			} else if expiry.ProducedAt == nil || !expiry.ProducedAt.Equal(produced.Truncate(time.Microsecond)) {
				t.Fatalf("unexpected production date: %v", expiry.ProducedAt)
			}
			if tt.source == model.ExpiryExplicit {
				if expiry.ShelfLifeHours != nil || !expiry.ExpiresAt.Equal(expires.Truncate(time.Microsecond)) {
					t.Fatalf("unexpected explicit expiry: %+v", expiry)
				}
				return
			}
			want := produced.Truncate(time.Microsecond).Add(time.Duration(tt.hours) * time.Hour)
			if expiry.ShelfLifeHours == nil || *expiry.ShelfLifeHours != tt.hours || !expiry.ExpiresAt.Equal(want) {
				t.Fatalf("unexpected calculated expiry: %+v", expiry)
			}
			if tt.standard != nil {
				*tt.standard = 12
				if *expiry.ShelfLifeHours != tt.hours || !expiry.ExpiresAt.Equal(want) {
					t.Fatal("prepared expiry changed with product standard")
				}
			}
		})
	}
}
