package forecast

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func parseFilter(raw string) (model.ForecastFilter, error) {
	f := model.ForecastFilter{LookbackDays: 7, HorizonHours: 72}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, &validations.FieldError{Field: "query", Message: "Некорректные параметры"}
	}
	if err := validations.ValidateQuery(q, "page", "page_size", "lookback_days", "horizon_hours"); err != nil {
		return f, err
	}
	p, err := validations.ParsePagination(q)
	if err != nil {
		return f, err
	}
	f.Page, f.PageSize = p.Page, p.PageSize
	for _, v := range []struct {
		name   string
		max    int
		target *int
	}{
		{"lookback_days", 90, &f.LookbackDays}, {"horizon_hours", 720, &f.HorizonHours},
	} {
		if values, ok := q[v.name]; ok {
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 || n > v.max {
				return f, &validations.FieldError{Field: v.name, Message: fmt.Sprintf("Допустимо от 1 до %d", v.max)}
			}
			*v.target = n
		}
	}
	return f, nil
}
