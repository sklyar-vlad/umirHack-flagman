package dish

import (
	"context"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
)

func (s *Service) Preview(ctx context.Context, input dto.PreviewDishRequest) (dto.DishPreview, error) {
	if _, err := actor(ctx); err != nil {
		return dto.DishPreview{}, err
	}
	in, err := validateRecipe(input.Recipe)
	if err != nil {
		return dto.DishPreview{}, err
	}
	var result dto.DishPreview
	err = s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			now := s.now().UTC()
			recipe, err := s.recipe(ctx, tx, in, now, false)
			if err != nil {
				return err
			}
			availability, err := menu.CalculateAvailability(true, recipe)
			if err != nil {
				return err
			}
			result = dto.DishPreview{
				IsAvailable:       availability.IsAvailable,
				AvailablePortions: availability.AvailablePortions,
				Shortages:         availability.Shortages,
				Requirements:      []dto.Requirement{},
				Meta:              map[string]any{"server_time": now.Format(time.RFC3339Nano)},
			}
			for _, item := range recipe {
				required, err := quantity.ParseMilli(item.QuantityPerPortion)
				if err != nil {
					return err
				}
				available, err := quantity.ParseMilli(item.AvailableQuantity)
				if err != nil {
					return err
				}
				missing := new(big.Int).Sub(required, available)
				if missing.Sign() < 0 {
					missing.SetInt64(0)
				}
				result.Requirements = append(
					result.Requirements,
					dto.Requirement{
						ProductID:         item.ProductID,
						ProductName:       item.ProductName,
						Unit:              item.Unit,
						RequiredQuantity:  quantity.FormatMilli(required),
						AvailableQuantity: quantity.FormatMilli(available),
						MissingQuantity:   quantity.FormatMilli(missing),
					},
				)
			}
			return nil
		},
	)
	if err != nil {
		return dto.DishPreview{}, databaseError(err)
	}
	return result, nil
}
