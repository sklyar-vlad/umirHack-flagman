package dish

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func snapshot(d model.Dish) *model.DishAuditSnapshot {
	result := &model.DishAuditSnapshot{Name: d.Name, Description: d.Description, IsActive: d.IsActive, Revision: int(d.Revision), RecipeVersionID: d.RecipeVersionID, RecipeVersion: d.RecipeVersion, Recipe: []model.RecipeIngredientAuditSnapshot{}}
	for _, item := range d.Recipe {
		result.Recipe = append(result.Recipe, model.RecipeIngredientAuditSnapshot{ProductID: item.ProductID, QuantityPerPortion: item.QuantityPerPortion})
	}
	return result
}

func (s *Service) changed(ctx context.Context, tx repository.DBTX, a model.Actor, before *model.Dish, after model.Dish) error {
	entry := &model.AuditEntry{ActorID: &a.ID, EntityType: model.AuditDish, EntityID: &after.ID, Action: model.AuditCreate, After: snapshot(after)}
	products := make([]uuid.UUID, 0, len(after.Recipe))
	if before != nil {
		entry.Action = model.AuditUpdate
		entry.Before = snapshot(*before)
		for _, item := range before.Recipe {
			products = append(products, item.ProductID)
		}
	}
	for _, item := range after.Recipe {
		products = append(products, item.ProductID)
	}
	slices.SortFunc(products, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	products = slices.Compact(products)
	if err := s.events.AppendAudit(ctx, tx, entry); err != nil {
		return err
	}
	return s.events.AppendOutbox(ctx, tx, &model.OutboxEvent{Type: model.DishesChanged, ActorID: &a.ID, DishIDs: []uuid.UUID{after.ID}, ProductIDs: products})
}
