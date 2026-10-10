package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"go.uber.org/zap"
)

func calculateBatchStatus(remainingQtyStr string, expiresAt, now time.Time, thresholdHours int) (string, int64) {
	// Floor the signed difference without overflowing time.Duration for distant dates.
	remSec := expiresAt.Unix() - now.Unix()
	if expiresAt.Nanosecond() < now.Nanosecond() {
		remSec--
	}

	// Если остаток 0 -> depleted
	if remainingQtyStr == "0.000" || remainingQtyStr == "0" {
		return string(model.BatchStatusDepleted), remSec
	}

	// Если срок истёк -> expired
	if !expiresAt.After(now) {
		return string(model.BatchStatusExpired), remSec
	}

	if !expiresAt.After(now.Add(time.Duration(thresholdHours) * time.Hour)) {
		return string(model.BatchStatusExpiring), remSec
	}

	return string(model.BatchStatusValid), remSec
}

func (s *service) ListSupplies(
	ctx context.Context,
	filter model.SupplyFilter,
) ([]supplydto.SupplySummaryDTO, int64, error) {
	supplies, total, err := s.repo.ListSupplies(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list supplies", zap.Error(err))
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
	details, err := s.repo.GetSupplyByID(ctx, id)
	if err != nil {
		if !errors.Is(err, apperrors.ErrNotFound) {
			s.logger.Error("failed to get supply by id", zap.Error(err), zap.String("id", id.String()))
		}
		return nil, err
	}
	if details == nil {
		return nil, apperrors.ErrNotFound
	}
	sup, items := &details.Supply, details.Items

	detailItems := make([]supplydto.SupplyItemDetailDTO, 0, len(items))
	for i := range items {
		it := &items[i]
		status, remSec := calculateBatchStatus(it.RemainingQuantity, it.ExpiresAt, now, details.ExpiringThresholdHours)

		var producedAtStr *string
		if it.ProducedAt != nil {
			val := it.ProducedAt.UTC().Format(time.RFC3339Nano)
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
					ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339Nano),
				},
				LineNumber:        it.LineNumber,
				InitialQuantity:   it.InitialQuantity,
				RemainingQuantity: it.RemainingQuantity,
				ProducedAt:        producedAtStr,
				ShelfLifeHours:    it.ShelfLifeHours,
				ExpirySource:      string(it.ExpirySource),
				ExpiresAt:         it.ExpiresAt.UTC().Format(time.RFC3339Nano),
				RemainingSeconds:  remSec,
				Status:            status,
				CreatedAt:         it.CreatedAt.UTC().Format(time.RFC3339Nano),
			},
		})
	}

	return &supplydto.SupplyDetailDTO{
		SupplySummaryDTO: supplydto.SupplySummaryDTO{
			ID:         sup.ID,
			Number:     sup.Number,
			CreatedAt:  sup.CreatedAt.UTC().Format(time.RFC3339Nano),
			ReceivedAt: sup.ReceivedAt.UTC().Format(time.RFC3339Nano),
			ItemsCount: sup.ItemsCount,
			CreatedBy: supplydto.ActorBriefDTO{
				ID:   sup.CreatedBy.ID,
				Name: sup.CreatedBy.Name,
			},
			Status: string(sup.Status),
		},
		Items: detailItems,
		Meta:  supplydto.SupplyDetailMetaDTO{ServerTime: now.UTC().Format(time.RFC3339Nano)},
	}, nil
}
