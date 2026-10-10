package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type historyRepositoryStub struct {
	order    model.OrderHistory
	attempts []model.OrderAttemptHistory
	err      error
	ctx      context.Context
	tx       repository.DBTX
}

func (r *historyRepositoryStub) ListOrders(
	ctx context.Context,
	tx repository.DBTX,
	_ model.OrderHistoryFilter,
) ([]model.OrderHistory, int64, error) {
	r.ctx, r.tx = ctx, tx
	return []model.OrderHistory{r.order}, 1, r.err
}

func (r *historyRepositoryStub) FindOrder(
	ctx context.Context,
	tx repository.DBTX,
	_ uuid.UUID,
) (model.OrderHistory, error) {
	r.ctx, r.tx = ctx, tx
	return r.order, r.err
}

func (r *historyRepositoryStub) ListAttempts(
	ctx context.Context,
	tx repository.DBTX,
	_ model.OrderHistoryFilter,
) ([]model.OrderAttemptHistory, int64, error) {
	r.ctx, r.tx = ctx, tx
	return r.attempts, 1, r.err
}

func TestHistoryUsesReadOnlySnapshotAndDiscardsFailedResults(t *testing.T) {
	failure := errors.New("test read/commit failure")
	for _, method := range []string{"orders", "details", "attempts"} {
		for _, source := range []string{"none", "repository", "transaction"} {
			t.Run(method+"/"+source, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				r := &historyRepositoryStub{
					order: model.OrderHistory{OrderRecord: model.OrderRecord{ID: uuid.New(), Number: 42}},
				}
				tx := &checkTransactions{}
				if source == "repository" {
					r.err = fmt.Errorf("read: %w", failure)
				}
				if source == "transaction" {
					tx.err = fmt.Errorf("commit: %w", failure)
				}
				s := NewHistoryService(r, tx)
				var err error
				switch method {
				case "orders":
					items, total, readErr := s.ListOrders(ctx, model.OrderHistoryFilter{})
					err = readErr
					if source != "none" && (items != nil || total != 0) {
						t.Fatal("failed list leaked data")
					}
					if source == "none" && (len(items) != 1 || items[0].Items == nil || items[0].Number != "42") {
						t.Fatal("invalid successful list")
					}
				case "details":
					item, readErr := s.GetOrder(ctx, r.order.ID)
					err = readErr
					if source != "none" && item.ID != uuid.Nil {
						t.Fatal("failed detail leaked data")
					}
					if source == "none" && (item.Items == nil || item.Number != "42") {
						t.Fatal("invalid successful detail")
					}
				case "attempts":
					items, total, readErr := s.ListAttempts(ctx, model.OrderHistoryFilter{})
					err = readErr
					if source != "none" && (items != nil || total != 0) {
						t.Fatal("failed attempts leaked data")
					}
					if source == "none" && items == nil {
						t.Fatal("empty attempts must be an array")
					}
				}
				if source == "none" && err != nil || source != "none" && !errors.Is(err, failure) {
					t.Fatalf("error propagation: %v", err)
				}
				if tx.calls != 1 || tx.options.IsoLevel != pgx.RepeatableRead ||
					tx.options.AccessMode != pgx.ReadOnly ||
					r.ctx != ctx ||
					r.tx != tx.tx {
					t.Fatalf("inconsistent read transaction: %+v", tx)
				}
			})
		}
	}
}

func TestHistoryAttemptSnapshotDecodeFailsWithoutPartialResponse(t *testing.T) {
	good := model.OrderAttemptHistory{
		OrderAttempt: model.OrderAttempt{
			ItemsSnapshot: json.RawMessage(`[]`),
			Shortages:     json.RawMessage(`[]`),
			Issues:        json.RawMessage(`[]`),
		},
	}
	for _, field := range []string{"items", "shortages", "issues"} {
		t.Run(field, func(t *testing.T) {
			bad := good
			switch field {
			case "items":
				bad.ItemsSnapshot = json.RawMessage(`{}`)
			case "shortages":
				bad.Shortages = json.RawMessage(`{}`)
			case "issues":
				bad.Issues = json.RawMessage(`{}`)
			}
			r := &historyRepositoryStub{attempts: []model.OrderAttemptHistory{good, bad}}
			items, total, err := NewHistoryService(
				r,
				&checkTransactions{},
			).ListAttempts(context.Background(), model.OrderHistoryFilter{})
			if err == nil || items != nil || total != 0 {
				t.Fatal("corrupt snapshot returned a partial success")
			}
		})
	}
	r := &historyRepositoryStub{attempts: []model.OrderAttemptHistory{good}}
	items, _, err := NewHistoryService(
		r,
		&checkTransactions{},
	).ListAttempts(context.Background(), model.OrderHistoryFilter{})
	if err != nil || len(items) != 1 || items[0].Items == nil || items[0].Shortages == nil || items[0].Issues == nil {
		t.Fatalf("empty snapshot arrays: %+v %v", items, err)
	}
}
