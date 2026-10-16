package supply

import (
	"testing"
	"time"

	batchservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/batch"
)

func TestCalculateBatchStatus(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 500000000, time.UTC)
	threshold := now.Add(48 * time.Hour)
	for _, test := range []struct {
		name     string
		quantity string
		expires  time.Time
		status   string
		seconds  int64
	}{
		{"threshold inclusive", "1.125", threshold, "expiring", 172800},
		{"after threshold", "1.125", threshold.Add(time.Nanosecond), "valid", 172800},
		{"exact expiry", "1.125", now, "expired", 0},
		{"just expired", "1.125", now.Add(-time.Nanosecond), "expired", -1},
		{"just before expiry", "1.125", now.Add(time.Nanosecond), "expiring", 0},
		{"depleted takes priority", "0.000", now.Add(-time.Hour), "depleted", -3600},
		{"zero without scale", "0", threshold, "depleted", 172800},
		{"distant date", "1.125", time.Date(9999, 12, 31, 12, 0, 0, 500000000, time.UTC), "valid", 251610624000},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, seconds := batchservice.CalculateStatus(test.quantity, test.expires, now, 48)
			if status != test.status || seconds != test.seconds {
				t.Fatalf("got %s, %d; want %s, %d", status, seconds, test.status, test.seconds)
			}
		})
	}
}
