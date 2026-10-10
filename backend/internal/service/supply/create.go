package supply

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

// Create receives the actor from the authenticated caller, never from input.
// It returns a result only after all records, audit and outbox have committed.
func (s *Service) Create(
	ctx context.Context,
	actorID uuid.UUID,
	input dto.CreateSupplyRequest,
) (model.CreatedSupply, error) {
	if actorID == uuid.Nil {
		return model.CreatedSupply{}, apperrors.ErrUnauthorized
	}

	var result model.CreatedSupply
	err := s.transactions.WithinTx(ctx, nil, func(tx repository.DBTX) error {
		var err error
		result, err = s.createInTx(ctx, tx, actorID, input)
		return err
	})
	if err != nil {
		return model.CreatedSupply{}, err
	}
	return result, nil
}

func (s *Service) createInTx(
	ctx context.Context,
	tx repository.DBTX,
	actorID uuid.UUID,
	input dto.CreateSupplyRequest,
) (model.CreatedSupply, error) {
	var result model.CreatedSupply
	// Validation reads and locks products in SQL ORDER BY id, then fixes T
	// and checks expiry against the state obtained after waiting for locks.
	basics, err := s.ValidateBasics(ctx, tx, input)
	if err != nil {
		return model.CreatedSupply{}, err
	}
	result = model.CreatedSupply{
		ServerTime: basics.ServerTime,
		NewSupply: model.NewSupply{
			ID: uuid.New(), Number: basics.Number,
			ReceivedAt: basics.ReceivedAt, CreatedBy: actorID,
		},
		Batches: make([]model.NewSupplyBatch, 0, len(basics.Items)),
	}
	result.CreatedAt, err = s.repo.InsertSupply(ctx, tx, result.NewSupply)
	if err != nil {
		return model.CreatedSupply{}, err
	}

	snapshot := &model.SupplyAuditSnapshot{
		Number: basics.Number, ReceivedAt: basics.ReceivedAt,
		Batches: make([]model.BatchAuditSnapshot, 0, len(basics.Items)),
	}
	productIDs := make([]uuid.UUID, 0, len(basics.Items))
	batchIDs := make([]uuid.UUID, 0, len(basics.Items))
	for i, line := range basics.Items {
		batch := model.NewSupplyBatch{
			ID: uuid.New(), SupplyID: result.ID, ProductID: line.ProductID,
			LineNumber: i + 1, QuantityMilli: line.QuantityMilli, Expiry: line.Expiry,
		}
		if err := s.repo.InsertBatch(ctx, tx, &batch); err != nil {
			return model.CreatedSupply{}, err
		}
		if err := s.repo.InsertReceipt(ctx, tx, &model.NewSupplyReceipt{
			BatchID: batch.ID, QuantityMilli: line.QuantityMilli,
			ProductName: line.ProductName, Unit: line.Unit,
			ActorID: actorID, OccurredAt: basics.ReceivedAt,
		}); err != nil {
			return model.CreatedSupply{}, err
		}
		quantity := fmt.Sprintf("%d.%03d", line.QuantityMilli/1000, line.QuantityMilli%1000)
		snapshot.Batches = append(snapshot.Batches, model.BatchAuditSnapshot{
			BatchID: batch.ID, ProductID: batch.ProductID, LineNumber: batch.LineNumber,
			InitialQuantity: quantity, RemainingQuantity: quantity,
			ProducedAt: line.Expiry.ProducedAt, ShelfLifeHours: line.Expiry.ShelfLifeHours,
			ExpirySource: string(line.Expiry.Source), ExpiresAt: line.Expiry.ExpiresAt,
		})
		result.Batches = append(result.Batches, batch)
		productIDs = append(productIDs, line.ProductID)
		batchIDs = append(batchIDs, batch.ID)
	}
	// Repeated lines for one product remain separate batches, but the event
	// invalidates each product once, in a deterministic order.
	slices.SortFunc(productIDs, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	productIDs = slices.Compact(productIDs)
	if err := s.transactions.AppendAudit(ctx, tx, &model.AuditEntry{
		ActorID: &actorID, Action: model.AuditCreate,
		EntityType: model.AuditSupply, EntityID: &result.ID, After: snapshot,
	}); err != nil {
		return model.CreatedSupply{}, err
	}
	if err := s.transactions.AppendOutbox(ctx, tx, &model.OutboxEvent{
		Type: model.InventoryChanged, ActorID: &actorID,
		ProductIDs: productIDs, BatchIDs: batchIDs, SupplyIDs: []uuid.UUID{result.ID},
	}); err != nil {
		return model.CreatedSupply{}, err
	}
	return result, nil
}
