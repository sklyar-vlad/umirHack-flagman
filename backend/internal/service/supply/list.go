package supply

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (s *Service) List(ctx context.Context, filter model.SupplyFilter) ([]supplydto.SupplySummaryDTO, int64, error) {
	var supplies []model.Supply
	var total int64
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := s.transactions.WithinTx(ctx, options, func(tx repository.DBTX) error {
		var err error
		supplies, total, err = s.repo.List(ctx, tx, filter)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]supplydto.SupplySummaryDTO, 0, len(supplies))
	for i := range supplies {
		sup := &supplies[i]
		result = append(result, supplydto.SupplySummaryDTO{
			ID:         sup.ID,
			Number:     sup.Number,
			CreatedAt:  sup.CreatedAt.UTC().Format(time.RFC3339Nano),
			ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339Nano),
			ItemsCount: sup.ItemsCount,
			CreatedBy:  supplydto.ActorBriefDTO{ID: sup.CreatedBy.ID, Name: sup.CreatedBy.Name},
			Status:     string(sup.Status),
		})
	}
	return result, total, nil
}
