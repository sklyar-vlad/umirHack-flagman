package dish

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

func (s *Service) Update(ctx context.Context, id uuid.UUID, input dto.UpdateDishRequest) (dto.Dish, error) {
	a, err := actor(ctx)
	if err != nil {
		return dto.Dish{}, err
	}
	if input.ExpectedRevision < 1 || input.ExpectedRevision >= 2147483647 {
		return dto.Dish{}, field("expected_revision")
	}
	if input.Name == nil && !input.Description.Set && input.IsActive == nil && input.Recipe == nil {
		return dto.Dish{}, field("body")
	}
	var recipe []model.RecipeIngredient
	if input.Recipe != nil {
		recipe, err = validateRecipe(*input.Recipe)
		if err != nil {
			return dto.Dish{}, err
		}
	}
	var result dto.Dish
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.LockActor(ctx, tx, a.ID); err != nil {
			return err
		}
		if err := s.repo.LockDish(ctx, tx, id); err != nil {
			return err
		}
		before, err := s.load(ctx, tx, id, s.now().UTC())
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
		if input.Description.Set {
			next.Description = input.Description.Value
		}
		if input.IsActive != nil {
			next.IsActive = *input.IsActive
		}
		if err := validateDish(&next); err != nil {
			return err
		}
		if input.Recipe != nil {
			if _, err := s.recipe(ctx, tx, recipe, s.now().UTC(), true); err != nil {
				return err
			}
		} else if next.IsActive {
			if len(before.Recipe) == 0 {
				return field("recipe")
			}
			if _, err := s.recipe(ctx, tx, before.Recipe, s.now().UTC(), true); err != nil {
				return err
			}
		}
		if err := s.repo.Update(ctx, tx, &next); err != nil {
			return err
		}
		if input.Recipe != nil {
			if err := s.repo.AddRecipe(ctx, tx, id, a.ID, recipe); err != nil {
				return err
			}
		}
		now := s.now().UTC()
		after, err := s.load(ctx, tx, id, now)
		if err != nil {
			return err
		}
		result, err = dishDTO(&after, now)
		if err != nil {
			return err
		}
		return s.changed(ctx, tx, a, &before, &after)
	})
	if err != nil {
		return dto.Dish{}, databaseError(err)
	}
	return result, nil
}
