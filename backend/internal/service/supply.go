package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
)

// Определение статуса партии согласно правилам складского учета
func calculateBatchStatus(remainingQtyStr string, expiresAt time.Time, now time.Time, shelfLifeHours *int) (string, int64) {
	remSec := int64(expiresAt.Sub(now).Seconds())
	
	// Если остаток 0 -> depleted
	qty, _ := strconv.ParseFloat(remainingQtyStr, 64)
	if qty <= 0 {
		return string(model.BatchStatusDepleted), remSec
	}

	// Если срок истёк -> expired
	if now.After(expiresAt) {
		return string(model.BatchStatusExpired), remSec
	}

	// Порог expiring (обычно последние 24/48ч или порог предупреждения по ТЗ/настройкам)
	// Например: осталось <= 24 часа
	if remSec <= 86400 {
		return string(model.BatchStatusExpiring), remSec
	}

	return string(model.BatchStatusValid), remSec
}

func (s *service) ListSupplies(ctx context.Context, filter model.SupplyFilter) ([]supplydto.SupplySummaryDTO, int64, error) {
	supplies, total, err := s.repo.ListSupplies(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list supplies", zap.Error(err))
		return nil, 0, err
	}

	result := make([]supplydto.SupplySummaryDTO, 0, len(supplies))
	for _, sup := range supplies {
		result = append(result, supplydto.SupplySummaryDTO{
			ID:         sup.ID,
			Number:     sup.Number,
			CreatedAt:  sup.CreatedAt.UTC().Format(time.RFC3339),
			ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339),
			ItemsCount: sup.ItemsCount,
			CreatedBy: supplydto.ActorBriefDTO{
				ID:   sup.CreatedBy.ID,
				Name: sup.CreatedBy.Name,
			},
			Status: string(sup.Status),
		})
	}

	return result, total, nil
}

func (s *service) GetSupplyByID(ctx context.Context, id uuid.UUID, now time.Time) (*supplydto.SupplyDetailDTO, error) {
	sup, items, err := s.repo.GetSupplyByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get supply by id", zap.Error(err), zap.String("id", id.String()))
		return nil, err
	}
	if sup == nil {
		return nil, apperrors.ErrNotFound
	}

	detailItems := make([]supplydto.SupplyItemDetailDTO, 0, len(items))
	for _, it := range items {
		status, remSec := calculateBatchStatus(it.RemainingQuantity, it.ExpiresAt, now, it.ShelfLifeHours)

		var producedAtStr *string
		if it.ProducedAt != nil {
			val := it.ProducedAt.UTC().Format(time.RFC3339)
			producedAtStr = &val
		}

		detailItems = append(detailItems, supplydto.SupplyItemDetailDTO{
			Product: supplydto.ProductBriefDTO{
				ID:          it.ProductID,
				Name:        it.ProductName,
				Unit:        it.Unit,
				DisplayUnit: it.DisplayUnit,
			},
			Batch: supplydto.BatchDetailDTO{
				ID:        it.BatchID,
				ProductID: it.ProductID,
				Unit:      it.Unit,
				Supply: supplydto.SupplyBatchParentDTO{
					ID:         sup.ID,
					Number:     sup.Number,
					ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339),
				},
				LineNumber:        it.LineNumber,
				InitialQuantity:   it.InitialQuantity,
				RemainingQuantity: it.RemainingQuantity,
				ProducedAt:        producedAtStr,
				ShelfLifeHours:    it.ShelfLifeHours,
				ExpirySource:      string(it.ExpirySource),
				ExpiresAt:         it.ExpiresAt.UTC().Format(time.RFC3339),
				RemainingSeconds:  remSec,
				Status:            status,
				CreatedAt:         it.CreatedAt.UTC().Format(time.RFC3339),
			},
		})
	}

	return &supplydto.SupplyDetailDTO{
		SupplySummaryDTO: supplydto.SupplySummaryDTO{
			ID:         sup.ID,
			Number:     sup.Number,
			CreatedAt:  sup.CreatedAt.UTC().Format(time.RFC3339),
			ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339),
			ItemsCount: sup.ItemsCount,
			CreatedBy: supplydto.ActorBriefDTO{
				ID:   sup.CreatedBy.ID,
				Name: sup.CreatedBy.Name,
			},
			Status: string(sup.Status),
		},
		Items: detailItems,
	}, nil
}