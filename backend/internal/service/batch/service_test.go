package batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
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
	exists       bool
	listCalls    int
	now, warning time.Time
	err          error
}

func (r *repositoryStub) ProductExists(context.Context, repository.DBTX, uuid.UUID) (bool, error) {
	return r.exists, nil
}

func (r *repositoryStub) ExpiringThresholdHours(context.Context, repository.DBTX) (int, error) {
	return 48, nil
}

func (r *repositoryStub) List(
	_ context.Context,
	_ repository.DBTX,
	_ model.BatchFilter,
	now, warning time.Time,
) ([]model.Batch, int64, error) {
	r.listCalls++
	r.now, r.warning = now, warning
	return []model.Batch{}, 0, r.err
}

func TestBatchListSingleClockNotFoundAndTransactionErrors(t *testing.T) {
	now := time.Date(2026, 10, 10, 15, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))
	failure := errors.New("database failure")
	for _, test := range []struct {
		name          string
		exists        bool
		repositoryErr error
		commitErr     error
		wantErr       error
	}{
		{"empty product", true, nil, nil, nil},
		{"missing product", false, nil, nil, apperrors.ErrNotFound},
		{"read failure", true, failure, nil, failure},
		{"commit failure", true, nil, failure, failure},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &repositoryStub{exists: test.exists, err: test.repositoryErr}
			tx := &transactionStub{err: test.commitErr}
			s := NewService(repo, tx)
			calls := 0
			s.now = func() time.Time { calls++; return now }
			result, err := s.ListBatches(
				context.Background(),
				model.BatchFilter{ProductID: uuid.New(), Page: 1, PageSize: 50},
			)
			if calls != 1 || tx.options.IsoLevel != pgx.RepeatableRead || tx.options.AccessMode != pgx.ReadOnly {
				t.Fatal("batch list must use one time and one read-only repeatable-read transaction")
			}
			if !test.exists {
				if repo.listCalls != 0 {
					t.Fatal("missing product was treated as an empty batch list")
				}
			} else if !repo.now.Equal(now) || repo.now.Location() != time.UTC || !repo.warning.Equal(now.Add(48*time.Hour)) {
				t.Fatal("list did not use the configured threshold and UTC clock")
			}
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || result != nil {
					t.Fatalf("failure must never return a partial list: %+v, %v", result, err)
				}
				return
			}
			if err != nil || result.Items == nil || result.Meta["server_time"] != now.UTC().Format(time.RFC3339Nano) {
				t.Fatalf("empty envelope/clock: %+v, %v", result, err)
			}
		})
	}
}
