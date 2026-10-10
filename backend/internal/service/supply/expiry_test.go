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

func expiryPointer[T any](value T) *T { return &value }

func TestResolveExpiry(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	received := now.Add(-time.Hour)
	produced := received.Add(-24 * time.Hour).Format(time.RFC3339)
	future := now.Add(time.Hour).Format(time.RFC3339)
	standard := 72
	tests := []struct {
		name     string
		item     dto.CreateSupplyItem
		standard *int
		source   model.ExpirySource
		hours    int
		field    string
		required bool
	}{
		{name: "explicit", item: dto.CreateSupplyItem{ExpiresAt: &future}, source: model.ExpiryExplicit},
		{
			name:   "explicit with production",
			item:   dto.CreateSupplyItem{ExpiresAt: &future, ProducedAt: &produced},
			source: model.ExpiryExplicit,
		},
		{
			name:     "explicit ignores standard",
			item:     dto.CreateSupplyItem{ExpiresAt: &future},
			standard: &standard,
			source:   model.ExpiryExplicit,
		},
		{
			name:     "calculated override",
			item:     dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(48)},
			standard: &standard,
			source:   model.ExpiryCalculated,
			hours:    48,
		},
		{
			name:     "calculated standard",
			item:     dto.CreateSupplyItem{ProducedAt: &produced},
			standard: &standard,
			source:   model.ExpiryCalculated,
			hours:    72,
		},
		{
			name:   "large duration without overflow",
			item:   dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(3000000)},
			source: model.ExpiryCalculated,
			hours:  3000000,
		},
		{
			name:  "mixed modes",
			item:  dto.CreateSupplyItem{ExpiresAt: &future, ShelfLifeHours: expiryPointer(72)},
			field: "items[0].shelf_life_hours",
		},
		{name: "no dates", field: "items[0].produced_at"},
		{
			name:     "missing shelf life",
			item:     dto.CreateSupplyItem{ProducedAt: &produced},
			field:    "items[0].shelf_life_hours",
			required: true,
		},
		{
			name:     "zero hours",
			item:     dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(0)},
			standard: &standard,
			field:    "items[0].shelf_life_hours",
		},
		{
			name:  "negative hours",
			item:  dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(-1)},
			field: "items[0].shelf_life_hours",
		},
		{
			name:  "outside RFC3339",
			item:  dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(2147483647)},
			field: "items[0].shelf_life_hours",
		},
		{
			name:  "production after receipt",
			item:  dto.CreateSupplyItem{ExpiresAt: &future, ProducedAt: expiryPointer(now.Format(time.RFC3339))},
			field: "items[0].produced_at",
		},
		{
			name:  "expiry equals receipt",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer(received.Format(time.RFC3339))},
			field: "items[0].expires_at",
		},
		{
			name:  "expiry equals now",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer(now.Format(time.RFC3339))},
			field: "items[0].expires_at",
		},
		{
			name:  "expired after receipt",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer(now.Add(-time.Minute).Format(time.RFC3339))},
			field: "items[0].expires_at",
		},
		{
			name:  "calculated expired",
			item:  dto.CreateSupplyItem{ProducedAt: &produced, ShelfLifeHours: expiryPointer(24)},
			field: "items[0].expires_at",
		},
		{name: "empty expiry", item: dto.CreateSupplyItem{ExpiresAt: expiryPointer("")}, field: "items[0].expires_at"},
		{
			name:  "missing timezone",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer("2026-10-10T13:00:00")},
			field: "items[0].expires_at",
		},
		{
			name:  "zero year",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer("0000-01-01T00:00:00Z")},
			field: "items[0].expires_at",
		},
		{
			name:  "submicrosecond expiry rejected",
			item:  dto.CreateSupplyItem{ExpiresAt: expiryPointer(now.Add(time.Nanosecond).Format(time.RFC3339Nano))},
			field: "items[0].expires_at",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveExpiry(
				"items[0]",
				tt.item,
				model.SupplyProduct{StandardShelfLifeHours: tt.standard},
				received,
				now,
			)
			if tt.field != "" {
				var field *validations.FieldError
				if !errors.As(err, &field) || field.Field != tt.field {
					t.Fatalf("expected field %s, got %v", tt.field, err)
				}
				if tt.required && !errors.Is(err, apperrors.ErrShelfLifeRequired) {
					t.Fatalf("missing required sentinel: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Source != tt.source {
				t.Fatalf("source: %s", result.Source)
			}
			if tt.source == model.ExpiryExplicit {
				if result.ShelfLifeHours != nil || !result.ExpiresAt.Equal(now.Add(time.Hour)) {
					t.Fatalf("explicit result: %+v", result)
				}
			} else {
				if result.ShelfLifeHours == nil || *result.ShelfLifeHours != tt.hours {
					t.Fatalf("hours: %+v", result)
				}
				if result.ShelfLifeHours == tt.standard || result.ShelfLifeHours == tt.item.ShelfLifeHours {
					t.Fatal("hours must be a copy")
				}
				want := received.AddDate(0, 0, -1).
					AddDate(0, 0, tt.hours/24).
					Add(time.Duration(tt.hours%24) * time.Hour)
				if !result.ExpiresAt.Equal(want) {
					t.Fatalf("expires %s, want %s", result.ExpiresAt, want)
				}
			}
		})
	}
}

func TestExpirySnapshotSurvivesStandardChange(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	hours := 72
	produced := now.Add(-24 * time.Hour).Format(time.RFC3339)
	expiry, err := resolveExpiry(
		"items[0]",
		dto.CreateSupplyItem{ProducedAt: &produced},
		model.SupplyProduct{StandardShelfLifeHours: &hours},
		now,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	hours = 12
	if *expiry.ShelfLifeHours != 72 || !expiry.ExpiresAt.Equal(now.Add(48*time.Hour)) {
		t.Fatalf("snapshot changed: %+v", expiry)
	}
}

func TestValidateBasicsRejectsOneExpiredLine(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	repo := &fakeRepository{products: map[uuid.UUID]model.SupplyProduct{id: {ID: id, Unit: "g", IsActive: true}}}
	s := NewService(repo, nil, nil)
	s.now = func() time.Time { return now }
	good := now.Add(time.Hour).Format(time.RFC3339)
	bad := now.Add(-time.Minute).Format(time.RFC3339)
	result, err := s.ValidateBasics(
		context.Background(),
		nil,
		dto.CreateSupplyRequest{
			Number:     "107",
			ReceivedAt: now.Add(-time.Hour).Format(time.RFC3339),
			Items: []dto.CreateSupplyItem{
				{ProductID: id.String(), Quantity: "1", ExpiresAt: &good},
				{ProductID: id.String(), Quantity: "1", ExpiresAt: &bad},
			},
		},
	)
	var field *validations.FieldError
	if !errors.As(err, &field) || field.Field != "items[1].expires_at" || len(result.Items) != 0 {
		t.Fatalf("expected whole request rejection, result=%+v err=%v", result, err)
	}
}
