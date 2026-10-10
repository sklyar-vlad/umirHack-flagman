package supply

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (s *Service) Receive(ctx context.Context, actorID, key uuid.UUID, body []byte) (model.IdempotencyResponse, error) {
	return s.CreateIdempotent(
		ctx,
		actorID,
		key,
		body,
		func(ctx context.Context, tx repository.DBTX, created *model.CreatedSupply) (json.RawMessage, error) {
			details, err := s.repo.FindDetails(ctx, tx, created.ID)
			if err != nil {
				return nil, err
			}
			return json.Marshal(makeSupplyDetail(details, created.ServerTime))
		},
	)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (dto.SupplyDetail, error) {
	var result dto.SupplyDetail
	options := &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := s.transactions.WithinTx(ctx, options, func(tx repository.DBTX) error {
		now := s.now().UTC()
		details, err := s.repo.FindDetails(ctx, tx, id)
		if err != nil {
			return err
		}
		result = makeSupplyDetail(details, now)
		return nil
	})
	if err != nil {
		return dto.SupplyDetail{}, err
	}
	return result, nil
}

func makeSupplyDetail(details *model.SupplyDetails, now time.Time) dto.SupplyDetail {
	reference := dto.SupplyReference{ID: details.ID, Number: details.Number, ReceivedAt: details.ReceivedAt.UTC()}
	result := dto.SupplyDetail{
		ID:         details.ID,
		Number:     details.Number,
		ReceivedAt: details.ReceivedAt.UTC(),
		CreatedAt:  details.CreatedAt.UTC(),
		CreatedBy:  dto.ActorBrief{ID: details.CreatedBy, Name: details.ActorName},
		Status:     "received",
		ItemsCount: len(details.Items),
		Items:      make([]dto.SupplyDetailItem, 0, len(details.Items)),
		Meta:       dto.SupplyMeta{ServerTime: now.UTC()},
	}
	for i := range details.Items {
		item := &details.Items[i]
		expires := item.Expiry.ExpiresAt.UTC()
		var produced *time.Time
		if item.Expiry.ProducedAt != nil {
			value := item.Expiry.ProducedAt.UTC()
			produced = &value
		}
		status, seconds := calculateBatchStatus(item.RemainingQuantity, expires, now, details.ExpiringThresholdHours)
		result.Items = append(result.Items, dto.SupplyDetailItem{
			Product: dto.ProductBrief{
				ID:          item.ProductID,
				Name:        item.ProductName,
				Unit:        item.Unit,
				DisplayUnit: item.DisplayUnit,
			},
			Batch: dto.SupplyBatch{
				ID:                item.ID,
				ProductID:         item.ProductID,
				Unit:              item.Unit,
				Supply:            reference,
				LineNumber:        item.LineNumber,
				InitialQuantity:   item.InitialQuantity,
				RemainingQuantity: item.RemainingQuantity,
				ProducedAt:        produced,
				ShelfLifeHours:    item.Expiry.ShelfLifeHours,
				ExpirySource:      string(item.Expiry.Source),
				ExpiresAt:         expires,
				RemainingSeconds:  seconds,
				Status:            status,
				CreatedAt:         item.CreatedAt.UTC(),
			},
		})
	}
	return result
}

func calculateBatchStatus(remainingQtyStr string, expiresAt, now time.Time, thresholdHours int) (string, int64) {
	// Floor the signed difference without overflowing time.Duration for distant dates.
	remSec := expiresAt.Unix() - now.Unix()
	if expiresAt.Nanosecond() < now.Nanosecond() {
		remSec--
	}

	// Если остаток 0 -> depleted
	if remainingQtyStr == "0.000" || remainingQtyStr == "0" {
		return "depleted", remSec
	}

	// Если срок истёк -> expired
	if !expiresAt.After(now) {
		return "expired", remSec
	}

	if !expiresAt.After(now.Add(time.Duration(thresholdHours) * time.Hour)) {
		return "expiring", remSec
	}

	return "valid", remSec
}
