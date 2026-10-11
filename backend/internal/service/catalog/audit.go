package catalog

import (
	"context"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func categorySnapshot(c model.CatalogCategory) *model.CategoryAuditSnapshot {
	return &model.CategoryAuditSnapshot{Name: c.Name, Revision: c.Revision}
}

func productSnapshot(p model.Product) *model.ProductAuditSnapshot {
	return &model.ProductAuditSnapshot{Name: p.Name, CategoryID: p.Category.ID, Unit: p.Unit, DisplayUnit: p.DisplayUnit, StandardShelfLifeHours: p.StandardShelfLifeHours, TargetStock: p.TargetStock, IsActive: p.IsActive, Revision: int(p.Revision)}
}

func (s *Service) categoryAudit(ctx context.Context, tx repository.DBTX, before *model.CatalogCategory, after model.CatalogCategory) error {
	a, err := actor(ctx, true)
	if err != nil {
		return err
	}
	entry := &model.AuditEntry{ActorID: &a.ID, EntityType: model.AuditProductCategory, EntityID: &after.ID, Action: model.AuditCreate, After: categorySnapshot(after)}
	if before != nil {
		entry.Action = model.AuditUpdate
		entry.Before = categorySnapshot(*before)
	}
	return s.events.AppendAudit(ctx, tx, entry)
}

func (s *Service) productAudit(ctx context.Context, tx repository.DBTX, before *model.Product, after model.Product) error {
	a, err := actor(ctx, true)
	if err != nil {
		return err
	}
	entry := &model.AuditEntry{ActorID: &a.ID, EntityType: model.AuditProduct, EntityID: &after.ID, Action: model.AuditCreate, After: productSnapshot(after)}
	if before != nil {
		entry.Action = model.AuditUpdate
		entry.Before = productSnapshot(*before)
	}
	return s.events.AppendAudit(ctx, tx, entry)
}
