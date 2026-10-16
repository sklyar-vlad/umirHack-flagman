package forecast

import (
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const fieldLookbackDays = "lookback_days"

type productUnit struct {
	id   uuid.UUID
	unit string
}

func consumptionRates(totals []model.ConsumptionTotal, days int) (map[productUnit]*big.Rat, error) {
	if days < 1 || days > 90 {
		return nil, &validations.FieldError{Field: fieldLookbackDays, Message: "Допустимо от 1 до 90 дней"}
	}
	period := big.NewInt(int64(time.Duration(days) * 24 * time.Hour))
	rates := map[productUnit]*big.Rat{}
	for _, v := range totals {
		n, err := quantity.ParseMilli(v.Quantity)
		if err != nil {
			return nil, fmt.Errorf("forecast consumption: %w", err)
		}
		if n.Sign() == 0 {
			continue
		}
		key := productUnit{v.ProductID, v.Unit}
		if rates[key] == nil {
			rates[key] = new(big.Rat)
		}
		rates[key].Add(rates[key], new(big.Rat).SetFrac(n, period))
	}
	return rates, nil
}

func validateFilter(f model.ForecastFilter) error {
	if f.LookbackDays < 1 || f.LookbackDays > 90 {
		return &validations.FieldError{Field: fieldLookbackDays, Message: "Допустимо от 1 до 90 дней"}
	}
	if f.HorizonHours < 1 || f.HorizonHours > 720 {
		return &validations.FieldError{Field: "horizon_hours", Message: "Допустимо от 1 до 720 часов"}
	}
	_, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	return err
}
