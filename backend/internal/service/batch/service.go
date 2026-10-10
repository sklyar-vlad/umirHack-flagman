package batch

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository interface {
	ProductExists(context.Context, repository.DBTX, uuid.UUID) (bool, error)
	ExpiringThresholdHours(context.Context, repository.DBTX) (int, error)
	List(context.Context, repository.DBTX, model.BatchFilter, time.Time, time.Time) ([]model.Batch, int64, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	now          func() time.Time
}

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions, now: time.Now}
}

func (s *Service) ListBatches(ctx context.Context, filter model.BatchFilter) (*dto.BatchListResponse, error) {
	var batches []model.Batch
	var total int64
	var hours int
	var now time.Time
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := s.transactions.WithinTx(ctx, options, func(tx repository.DBTX) error {
		now = s.now().UTC()
		exists, err := s.repo.ProductExists(ctx, tx, filter.ProductID)
		if err != nil {
			return err
		}
		if !exists {
			return apperrors.ErrNotFound
		}
		hours, err = s.repo.ExpiringThresholdHours(ctx, tx)
		if err != nil {
			return err
		}
		batches, total, err = s.repo.List(ctx, tx, filter, now, now.Add(time.Duration(hours)*time.Hour))
		return err
	})
	if err != nil {
		return nil, err
	}
	items := make([]dto.SupplyBatch, 0, len(batches))
	for i := range batches {
		b := &batches[i]
		expires := b.Expiry.ExpiresAt.UTC()
		var produced *time.Time
		if b.Expiry.ProducedAt != nil {
			value := b.Expiry.ProducedAt.UTC()
			produced = &value
		}
		status, seconds := CalculateStatus(b.RemainingQuantity, expires, now, hours)
		items = append(items, dto.SupplyBatch{
			ID: b.ID, ProductID: b.ProductID, Unit: b.Unit,
			Supply:     dto.SupplyReference{ID: b.SupplyID, Number: b.SupplyNumber, ReceivedAt: b.ReceivedAt.UTC()},
			LineNumber: b.LineNumber, InitialQuantity: b.InitialQuantity, RemainingQuantity: b.RemainingQuantity,
			ProducedAt: produced, ShelfLifeHours: b.Expiry.ShelfLifeHours, ExpirySource: string(b.Expiry.Source),
			ExpiresAt: expires, RemainingSeconds: seconds, Status: status, CreatedAt: b.CreatedAt.UTC(),
		})
	}
	return &dto.BatchListResponse{ListResponse: dto.ListResponse[dto.SupplyBatch]{
		Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize,
		Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)},
	}}, nil
}
