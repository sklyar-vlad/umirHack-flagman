package analytics

import (
	"context"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const (
	queryFrom = "from"
	queryTo   = "to"
)

type Service interface {
	ProductFlow(context.Context, model.AnalyticsFilter) ([]dto.ProductFlowAnalytics, int64, error)
	IngredientShortages(context.Context, model.AnalyticsFilter) ([]dto.IngredientShortageAnalytics, int64, error)
	WriteOffs(context.Context, model.AnalyticsFilter) ([]dto.WriteOffAnalytics, int64, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) ProductFlow(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	q := c.Request.URL.Query()
	if _, ok := q["reason"]; ok {
		basehandler.WriteError(
			c,
			&validations.FieldError{Field: "reason", Message: "Для движения продуктов фильтр причины не используется"},
		)
		return
	}
	f, err := parseFilter(c.Request.URL.RawQuery, true)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.ProductFlow(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		200,
		dto.ListResponse[dto.ProductFlowAnalytics]{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
	)
}

func (h *Handler) WriteOffs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseFilter(c.Request.URL.RawQuery, true)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.WriteOffs(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		200,
		dto.ListResponse[dto.WriteOffAnalytics]{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
	)
}

func parseFilter(raw string, losses bool) (model.AnalyticsFilter, error) {
	var f model.AnalyticsFilter
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, &validations.FieldError{Field: "query", Message: "Некорректные параметры"}
	}
	allowed := []string{"page", "page_size", queryFrom, queryTo}
	if losses {
		allowed = append(allowed, "product_id", "reason")
	}
	if err := validations.ValidateQuery(q, allowed...); err != nil {
		return f, err
	}
	p, err := validations.ParsePagination(q)
	if err != nil {
		return f, err
	}
	f.Page, f.PageSize = p.Page, p.PageSize
	if losses {
		f.Reason, err = validations.ParseEnum(q, "reason", "all", "all", "expiration", "manual_writeoff")
		if err != nil {
			return f, err
		}
		if values, ok := q["product_id"]; ok {
			id, err := validations.ParseUUID("product_id", values[0])
			if err != nil {
				return f, err
			}
			f.ProductID = &id
		}
	}
	for _, name := range []string{queryFrom, queryTo} {
		if values, ok := q[name]; ok {
			v, err := validations.ParseDateTime(name, values[0])
			if err != nil {
				return f, err
			}
			if name == queryFrom {
				f.DateFrom = &v
			} else {
				f.DateTo = &v
			}
		}
	}
	if f.DateFrom != nil && f.DateTo != nil && f.DateFrom.After(*f.DateTo) {
		return f, &validations.FieldError{Field: queryTo, Message: "Конец интервала не может быть раньше начала"}
	}
	return f, nil
}

func (h *Handler) IngredientShortages(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseFilter(c.Request.URL.RawQuery, false)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.IngredientShortages(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		200,
		dto.ListResponse[dto.IngredientShortageAnalytics]{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
	)
}
