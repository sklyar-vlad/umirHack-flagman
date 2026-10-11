package writeoff

import (
	"context"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type HistoryService interface {
	List(context.Context, model.StockMovementFilter) ([]dto.StockMovement, int64, error)
}
type HistoryHandler struct{ service HistoryService }

func NewHistoryHandler(service HistoryService) *HistoryHandler {
	return &HistoryHandler{service: service}
}

func (h *HistoryHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseFilter(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		200,
		dto.ListResponse[dto.StockMovement]{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
	)
}

func parseFilter(raw string) (model.StockMovementFilter, error) {
	var f model.StockMovementFilter
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, &validations.FieldError{Field: "query", Message: "Некорректные параметры"}
	}
	if err := validations.ValidateQuery(
		q,
		"page",
		"page_size",
		"sort",
		"product_id",
		"batch_id",
		"order_id",
		"type",
		"from",
		"to",
	); err != nil {
		return f, err
	}
	p, err := validations.ParsePagination(q)
	if err != nil {
		return f, err
	}
	f.Page, f.PageSize = p.Page, p.PageSize
	if _, err = validations.ParseSort(q, "-occurred_at", "-occurred_at"); err != nil {
		return f, err
	}
	for name, dst := range map[string]**uuid.UUID{"product_id": &f.ProductID, "batch_id": &f.BatchID, "order_id": &f.OrderID} {
		if values, ok := q[name]; ok {
			id, err := validations.ParseUUID(name, values[0])
			if err != nil {
				return f, err
			}
			*dst = &id
		}
	}
	if _, ok := q["type"]; ok {
		f.Type, err = validations.ParseEnum(
			q,
			"type",
			"",
			"receipt",
			"order_consumption",
			"expiration",
			"manual_writeoff",
		)
		if err != nil {
			return f, err
		}
	}
	for _, name := range []string{"from", "to"} {
		if values, ok := q[name]; ok {
			v, err := validations.ParseDateTime(name, values[0])
			if err != nil {
				return f, err
			}
			if name == "from" {
				f.DateFrom = &v
			} else {
				f.DateTo = &v
			}
		}
	}
	if f.DateFrom != nil && f.DateTo != nil && f.DateFrom.After(*f.DateTo) {
		return f, &validations.FieldError{Field: "to", Message: "Конец интервала не может быть раньше начала"}
	}
	return f, nil
}
