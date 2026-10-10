package inventory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository interface {
	ExpiringThresholdHours(context.Context, repository.DBTX) (int, error)
	List(context.Context, repository.DBTX, model.InventoryFilter, time.Time, time.Time) (*model.InventoryResult, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	now          func() time.Time
}

const zeroQuantity = "0.000"

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions, now: time.Now}
}

func (s *Service) ListInventory(ctx context.Context, filter model.InventoryFilter) (*dto.InventoryListResponse, error) {
	var result *model.InventoryResult
	var now time.Time
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := s.transactions.WithinTx(ctx, options, func(tx repository.DBTX) error {
		now = s.now().UTC()
		hours, err := s.repo.ExpiringThresholdHours(ctx, tx)
		if err != nil {
			return err
		}
		result, err = s.repo.List(ctx, tx, filter, now, now.Add(time.Duration(hours)*time.Hour))
		return err
	})
	if err != nil {
		return nil, err
	}
	items := make([]dto.InventoryItem, 0, len(result.Items))
	for i := range result.Items {
		it := &result.Items[i]
		p := &it.Product
		item := dto.InventoryItem{
			Product: dto.InventoryProduct{
				ProductBrief: dto.ProductBrief{ID: p.ID, Name: p.Name, Unit: p.Unit, DisplayUnit: p.DisplayUnit},
				Category:     dto.CategoryBrief{ID: p.Category.ID, Name: p.Category.Name}, IsActive: p.IsActive,
			},
			PhysicalQuantity: it.PhysicalQuantity, AvailableQuantity: it.AvailableQuantity,
			ExpiringQuantity: it.ExpiringQuantity, ExpiredQuantity: it.ExpiredQuantity,
			TargetStock: it.TargetStock, DeficitQuantity: it.DeficitQuantity, BatchesCount: it.BatchesCount,
			HasExpiring: it.ExpiringQuantity != zeroQuantity, HasExpired: it.ExpiredQuantity != zeroQuantity,
			IsOutOfStock: it.AvailableQuantity == zeroQuantity,
		}
		switch {
		case item.HasExpired:
			item.Status = "expired"
		case item.HasExpiring:
			item.Status = "expiring"
		case item.IsOutOfStock:
			item.Status = "out_of_stock"
		default:
			item.Status = "normal"
		}
		if it.NearestExpiresAt != nil {
			value := it.NearestExpiresAt.UTC().Format(time.RFC3339Nano)
			seconds := remainingSeconds(*it.NearestExpiresAt, now)
			item.NearestExpiresAt = &value
			item.RemainingSeconds = &seconds
		}
		items = append(items, item)
	}
	attention := make([]dto.AttentionBatch, 0, len(result.AttentionBatches))
	for i := range result.AttentionBatches {
		b := &result.AttentionBatches[i]
		status := "expiring"
		if !b.ExpiresAt.After(now) {
			status = "expired"
		}
		attention = append(attention, dto.AttentionBatch{
			BatchID:           b.BatchID,
			ProductID:         b.ProductID,
			ProductName:       b.ProductName,
			SupplyID:          b.SupplyID,
			SupplyNumber:      b.SupplyNumber,
			LineNumber:        b.LineNumber,
			RemainingQuantity: b.RemainingQuantity,
			Unit:              b.Unit,
			Status:            status,
			ExpiresAt:         b.ExpiresAt.UTC().Format(time.RFC3339Nano),
			RemainingSeconds:  remainingSeconds(b.ExpiresAt, now),
		})
	}
	return &dto.InventoryListResponse{
		ListResponse: dto.ListResponse[dto.InventoryItem]{
			Items: items, Total: result.Total, Page: filter.Page, PageSize: filter.PageSize,
			Meta: map[string]any{"server_time": now.Format(time.RFC3339Nano)},
		},
		Counts: dto.InventoryCounts{
			All: result.Counts.All, Expiring: result.Counts.Expiring, Expired: result.Counts.Expired,
			OutOfStock: result.Counts.OutOfStock, Deficit: result.Counts.Deficit,
		},
		AttentionBatches: attention, AttentionTotal: result.AttentionTotal,
	}, nil
}

func remainingSeconds(expires, now time.Time) int64 {
	seconds := expires.Unix() - now.Unix()
	if expires.Nanosecond() < now.Nanosecond() {
		seconds--
	}
	return seconds
}
