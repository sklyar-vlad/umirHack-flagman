package supply

import (
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestSupplyDetailExpiryBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 500000000, time.UTC)
	for _, test := range []struct {
		name, quantity, status string
		expires                time.Time
		seconds                int64
	}{
		{"threshold inclusive", "1.000", "expiring", now.Add(24 * time.Hour), 86400},
		{"above threshold", "1.000", "valid", now.Add(24*time.Hour + time.Nanosecond), 86400},
		{"expiry inclusive", "1.000", "expired", now, 0},
		{"negative fractional second", "1.000", "expired", now.Add(-time.Nanosecond), -1},
		{"positive fractional second", "1.000", "expiring", now.Add(time.Nanosecond), 0},
		{"depleted priority", "0.000", "depleted", now.Add(-time.Hour), -3600},
		{"distant expiry", "1.000", "valid", time.Date(9999, 12, 31, 0, 0, 0, 500000000, time.UTC), 251610580800},
	} {
		t.Run(test.name, func(t *testing.T) {
			details := &model.SupplyDetails{ExpiringThresholdHours: 24, Items: []model.SupplyItemDetails{
				{RemainingQuantity: test.quantity, Expiry: model.BatchExpiry{ExpiresAt: test.expires}},
			}}
			batch := makeSupplyDetail(details, now).Items[0].Batch
			if batch.Status != test.status || batch.RemainingSeconds != test.seconds {
				t.Fatalf(
					"status %s, seconds %d; want %s, %d",
					batch.Status,
					batch.RemainingSeconds,
					test.status,
					test.seconds,
				)
			}
		})
	}
}

func TestSupplyDetailUTCAndConfiguredThreshold(t *testing.T) {
	zone := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, zone)
	produced := now.Add(-time.Hour)
	details := &model.SupplyDetails{
		NewSupply: model.NewSupply{ReceivedAt: now}, CreatedAt: now,
		ExpiringThresholdHours: 48,
		Items: []model.SupplyItemDetails{{
			CreatedAt: now, RemainingQuantity: "1.000",
			Expiry: model.BatchExpiry{ProducedAt: &produced, ExpiresAt: now.Add(36 * time.Hour)},
		}},
	}
	result := makeSupplyDetail(details, now)
	batch := result.Items[0].Batch
	for _, value := range []time.Time{
		result.ReceivedAt, result.CreatedAt, result.Meta.ServerTime,
		batch.Supply.ReceivedAt, batch.CreatedAt, *batch.ProducedAt, batch.ExpiresAt,
	} {
		if value.Location() != time.UTC {
			t.Fatalf("date was not converted to UTC: %v", value)
		}
	}
	if produced.Location() != zone || batch.Status != "expiring" {
		t.Fatal("projection mutates source or ignores restaurant threshold")
	}
}
