package order

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (s *HistoryService) ListAttempts(ctx context.Context, filter model.OrderHistoryFilter) ([]dto.OrderAttempt, int64, error) {
	var attempts []model.OrderAttemptHistory
	var total int64
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		attempts, total, err = s.repo.ListAttempts(ctx, tx, filter)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.OrderAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		item := dto.OrderAttempt{ID: attempt.ID, CreatedAt: attempt.CreatedAt.UTC(), ReasonCode: attempt.ReasonCode,
			CreatedBy: dto.ActorBrief{ID: attempt.ActorID, Name: attempt.ActorName}}
		if err := json.Unmarshal(attempt.ItemsSnapshot, &item.Items); err != nil {
			return nil, 0, fmt.Errorf("decode order attempt items: %w", err)
		}
		if err := json.Unmarshal(attempt.Shortages, &item.Shortages); err != nil {
			return nil, 0, fmt.Errorf("decode order attempt shortages: %w", err)
		}
		if err := json.Unmarshal(attempt.Issues, &item.Issues); err != nil {
			return nil, 0, fmt.Errorf("decode order attempt issues: %w", err)
		}
		if item.Items == nil {
			item.Items = []dto.OrderItemSnapshot{}
		}
		if item.Shortages == nil {
			item.Shortages = []dto.Shortage{}
		}
		if item.Issues == nil {
			item.Issues = []dto.OrderIssue{}
		}
		result = append(result, item)
	}
	return result, total, nil
}
