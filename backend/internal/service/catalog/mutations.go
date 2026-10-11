package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (s *Service) CreateCategory(ctx context.Context, input dto.CreateCategoryRequest) (dto.CatalogCategory, error) {
	if _, err := actor(ctx, true); err != nil {
		return dto.CatalogCategory{}, err
	}
	value, err := name(input.Name)
	if err != nil {
		return dto.CatalogCategory{}, err
	}
	var c model.CatalogCategory
	err = s.write(ctx, func(tx repository.DBTX) error {
		var err error
		c, err = s.repo.CreateCategory(ctx, tx, value)
		if err != nil {
			return err
		}
		return s.categoryAudit(ctx, tx, nil, c)
	})
	if err != nil {
		return dto.CatalogCategory{}, err
	}
	return categoryDTO(c), nil
}

func (s *Service) UpdateCategory(ctx context.Context, id uuid.UUID, input dto.UpdateCategoryRequest) (dto.CatalogCategory, error) {
	if _, err := actor(ctx, true); err != nil {
		return dto.CatalogCategory{}, err
	}
	value, err := name(input.Name)
	if err != nil {
		return dto.CatalogCategory{}, err
	}
	if err := revision(input.ExpectedRevision); err != nil {
		return dto.CatalogCategory{}, err
	}
	var c model.CatalogCategory
	err = s.write(ctx, func(tx repository.DBTX) error {
		before, err := s.repo.GetCategory(ctx, tx, id, "update")
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return &apperrors.RevisionConflict{CurrentRevision: before.Revision}
		}
		next := before
		next.Name = value
		c, err = s.repo.UpdateCategory(ctx, tx, next)
		if err != nil {
			return err
		}
		return s.categoryAudit(ctx, tx, &before, c)
	})
	if err != nil {
		return dto.CatalogCategory{}, err
	}
	return categoryDTO(c), nil
}

func (s *Service) categoryReference(ctx context.Context, tx repository.DBTX, id uuid.UUID) error {
	_, err := s.repo.GetCategory(ctx, tx, id, "share")
	if errors.Is(err, apperrors.ErrNotFound) {
		return field("category_id")
	}
	return err
}

func (s *Service) CreateProduct(ctx context.Context, input dto.CreateProductRequest) (dto.Product, error) {
	if _, err := actor(ctx, true); err != nil {
		return dto.Product{}, err
	}
	p := model.Product{Name: input.Name, Category: model.ProductCategory{ID: input.CategoryID}, Unit: input.Unit, DisplayUnit: input.DisplayUnit, StandardShelfLifeHours: input.StandardShelfLifeHours, TargetStock: input.TargetStock, IsActive: true}
	if input.IsActive != nil {
		p.IsActive = *input.IsActive
	}
	if err := validateProduct(&p); err != nil {
		return dto.Product{}, err
	}
	err := s.write(ctx, func(tx repository.DBTX) error {
		if err := s.categoryReference(ctx, tx, p.Category.ID); err != nil {
			return err
		}
		var err error
		p, err = s.repo.CreateProduct(ctx, tx, p)
		if err != nil {
			return err
		}
		return s.productAudit(ctx, tx, nil, p)
	})
	if err != nil {
		return dto.Product{}, err
	}
	return productDTO(p), nil
}

func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, input dto.UpdateProductRequest) (dto.Product, error) {
	if _, err := actor(ctx, true); err != nil {
		return dto.Product{}, err
	}
	if err := revision(input.ExpectedRevision); err != nil {
		return dto.Product{}, err
	}
	if input.Name == nil && input.CategoryID == nil && input.Unit == nil && input.DisplayUnit == nil && !input.StandardShelfLifeHours.Set && !input.TargetStock.Set && input.IsActive == nil {
		return dto.Product{}, field("body")
	}
	var p model.Product
	err := s.write(ctx, func(tx repository.DBTX) error {
		if input.CategoryID != nil {
			if err := s.categoryReference(ctx, tx, *input.CategoryID); err != nil {
				return err
			}
		}
		before, err := s.repo.GetProduct(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if before.Revision != int64(input.ExpectedRevision) {
			return &apperrors.RevisionConflict{CurrentRevision: int(before.Revision)}
		}
		next := before
		if input.Name != nil {
			next.Name = *input.Name
		}
		if input.CategoryID != nil {
			next.Category.ID = *input.CategoryID
		}
		if input.Unit != nil {
			next.Unit = *input.Unit
		}
		if input.DisplayUnit != nil {
			next.DisplayUnit = *input.DisplayUnit
		}
		if input.StandardShelfLifeHours.Set {
			next.StandardShelfLifeHours = input.StandardShelfLifeHours.Value
		}
		if input.TargetStock.Set {
			next.TargetStock = input.TargetStock.Value
		}
		if input.IsActive != nil {
			next.IsActive = *input.IsActive
		}
		if err := validateProduct(&next); err != nil {
			return err
		}
		if before.Unit != next.Unit {
			used, err := s.repo.HasHistory(ctx, tx, id)
			if err != nil {
				return err
			}
			if used {
				return apperrors.ErrUnitLocked
			}
		}
		if before.IsActive && !next.IsActive {
			dishes, err := s.repo.ActiveDishes(ctx, tx, id)
			if err != nil {
				return err
			}
			if len(dishes) > 0 {
				return &apperrors.ProductInUse{Dishes: dishes}
			}
		}
		p, err = s.repo.UpdateProduct(ctx, tx, next)
		if err != nil {
			return err
		}
		return s.productAudit(ctx, tx, &before, p)
	})
	if err != nil {
		return dto.Product{}, err
	}
	return productDTO(p), nil
}
