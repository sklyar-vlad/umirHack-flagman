package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type transactionStub struct {
	options *pgx.TxOptions
	err     error
}

func (t *transactionStub) WithinTx(
	_ context.Context,
	options *pgx.TxOptions,
	fn func(repository.DBTX) error,
) error {
	t.options = options
	if err := fn(nil); err != nil {
		return err
	}
	return t.err
}

type repositoryStub struct {
	now, warning time.Time
	err          error
}

func (r *repositoryStub) ExpiringThresholdHours(context.Context, repository.DBTX) (int, error) {
	return 48, nil
}

func (r *repositoryStub) List(
	_ context.Context,
	_ repository.DBTX,
	_ model.InventoryFilter,
	now, warning time.Time,
) (*model.InventoryResult, error) {
	r.now, r.warning = now, warning
	return &model.InventoryResult{}, r.err
}

func TestInventorySingleClockAndTransactionErrors(t *testing.T) {
	now := time.Date(2026, 10, 10, 15, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))
	failure := errors.New("database failure")
	for _, test := range []struct {
		name          string
		repositoryErr error
		commitErr     error
	}{
		{"success", nil, nil},
		{"read failure", failure, nil},
		{"commit failure", nil, failure},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &repositoryStub{err: test.repositoryErr}
			tx := &transactionStub{err: test.commitErr}
			s := NewService(repo, tx)
			clockCalls := 0
			s.now = func() time.Time { clockCalls++; return now }
			result, err := s.ListInventory(context.Background(), model.InventoryFilter{Page: 1, PageSize: 50})
			if clockCalls != 1 || !repo.now.Equal(now) || repo.now.Location() != time.UTC ||
				!repo.warning.Equal(now.Add(48*time.Hour)) || tx.options.IsoLevel != pgx.RepeatableRead ||
				tx.options.AccessMode != pgx.ReadOnly {
				t.Fatal("inventory did not use a single UTC clock in a read-only repeatable-read transaction")
			}
			if test.repositoryErr != nil || test.commitErr != nil {
				if !errors.Is(err, failure) || result != nil {
					t.Fatal("transaction error must never return a partial inventory")
				}
				return
			}
			if err != nil || result.Items == nil || result.AttentionBatches == nil ||
				result.Meta["server_time"] != now.UTC().Format(time.RFC3339Nano) {
				t.Fatalf("empty envelope/clock: %+v, %v", result, err)
			}
		})
	}
}

func TestRemainingSecondsFloorsAndDoesNotOverflowDuration(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 500000000, time.UTC)
	for _, test := range []struct {
		name    string
		expires time.Time
		want    int64
	}{
		{"at expiry", now, 0},
		{"future fraction", now.Add(time.Nanosecond), 0},
		{"expired fraction", now.Add(-time.Nanosecond), -1},
		{"future seconds", now.Add(1500 * time.Millisecond), 1},
		{"expired seconds", now.Add(-1500 * time.Millisecond), -2},
		{"far future", time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), 251610580799},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := remainingSeconds(test.expires, now); got != test.want {
				t.Fatalf("seconds %d; want %d", got, test.want)
			}
		})
	}
}
