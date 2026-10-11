package forecast

import (
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
)

// BuildForecasts consumes one immutable snapshot. Product demand is warehouse
// demand, not multiplied independently by the number of dishes being forecast.
func BuildForecasts(dishes []model.Dish, batches []model.OrderBatch, totals []model.ConsumptionTotal, now time.Time, f model.ForecastFilter) ([]dto.StopListForecast, error) {
	if err := validateFilter(f); err != nil {
		return nil, err
	}
	rates, err := consumptionRates(totals, f.LookbackDays)
	if err != nil {
		return nil, err
	}
	byProduct := map[uuid.UUID][]model.OrderBatch{}
	for _, b := range batches {
		byProduct[b.ProductID] = append(byProduct[b.ProductID], b)
	}
	items := []dto.StopListForecast{}
	for _, dish := range dishes {
		if !dish.IsActive {
			continue
		}
		item := dto.StopListForecast{DishID: dish.ID, DishName: dish.Name, Cause: "unknown", Confidence: "low", Assumptions: []string{
			"estimate_only", "no_future_supplies", "current_recipe_unchanged", "constant_average_product_consumption_full_lookback", "continuous_expected_consumption",
		}}
		missing := []uuid.UUID{}
		for _, ingredient := range dish.Recipe {
			if rates[productUnit{ingredient.ProductID, ingredient.Unit}] == nil {
				missing = append(missing, ingredient.ProductID)
			}
		}
		slices.SortFunc(missing, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		if len(missing) > 0 || len(dish.Recipe) == 0 {
			item.Confidence = "insufficient_data"
			item.Assumptions = append(item.Assumptions, "expiry_only_for_missing_history")
			for _, id := range missing {
				item.Assumptions = append(item.Assumptions, "missing_history:"+id.String())
			}
		}
		availability, err := menu.CalculateAvailability(dish.IsActive, dish.Recipe)
		if err != nil {
			return nil, err
		}
		if !availability.IsAvailable {
			at := now.UTC()
			item.EstimatedStopAt = &at
			item.Cause = "already_unavailable"
			for _, v := range availability.Shortages {
				if item.LimitingProductID == nil || slices.Compare(v.ProductID[:], item.LimitingProductID[:]) < 0 {
					id := v.ProductID
					item.LimitingProductID = &id
				}
			}
			if len(dish.Recipe) == 0 {
				item.Assumptions = append(item.Assumptions, "recipe_missing")
			}
		} else {
			for _, ingredient := range dish.Recipe {
				required, err := quantity.ParseMilli(ingredient.QuantityPerPortion)
				if err != nil {
					return nil, err
				}
				at, cause, err := ingredientStop(byProduct[ingredient.ProductID], required, rates[productUnit{ingredient.ProductID, ingredient.Unit}], now, time.Duration(f.HorizonHours)*time.Hour)
				if err != nil {
					return nil, err
				}
				if at != nil && (item.EstimatedStopAt == nil || at.Before(*item.EstimatedStopAt) || at.Equal(*item.EstimatedStopAt) && slices.Compare(ingredient.ProductID[:], item.LimitingProductID[:]) < 0) {
					id := ingredient.ProductID
					item.EstimatedStopAt = at
					item.Cause = cause
					item.LimitingProductID = &id
				}
			}
			if item.EstimatedStopAt == nil {
				if item.Confidence == "insufficient_data" {
					item.Assumptions = append(item.Assumptions, "unknown_demand_no_expiry_stop_within_horizon")
				} else {
					item.Assumptions = append(item.Assumptions, "no_stop_within_horizon")
				}
			}
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b dto.StopListForecast) int {
		if a.EstimatedStopAt == nil && b.EstimatedStopAt != nil {
			return 1
		}
		if a.EstimatedStopAt != nil && b.EstimatedStopAt == nil {
			return -1
		}
		if a.EstimatedStopAt != nil {
			if v := a.EstimatedStopAt.Compare(*b.EstimatedStopAt); v != 0 {
				return v
			}
		}
		return slices.Compare(a.DishID[:], b.DishID[:])
	})
	return items, nil
}
