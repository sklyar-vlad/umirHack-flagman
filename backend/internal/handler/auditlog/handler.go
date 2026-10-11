package auditlog

import (
	"context"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	List(context.Context, *model.AuditLogFilter) ([]dto.AuditLogEntry, int64, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseFilter(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.List(c.Request.Context(), &f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		200,
		dto.ListResponse[dto.AuditLogEntry]{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
	)
}

func parseFilter(raw string) (model.AuditLogFilter, error) {
	var f model.AuditLogFilter
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, &validations.FieldError{Field: "query", Message: "Некорректные параметры"}
	}
	if err := validations.ValidateQuery(
		q,
		"page",
		"page_size",
		"search",
		"actor_id",
		"action",
		"entity_type",
		"entity_id",
		"from",
		"to",
		"sort",
	); err != nil {
		return f, err
	}
	page, err := validations.ParsePagination(q)
	if err != nil {
		return f, err
	}
	if _, err := validations.ParseSort(q, "-created_at", "-created_at"); err != nil {
		return f, err
	}
	f.Page = page.Page
	f.PageSize = page.PageSize
	f.Search = q.Get("search")
	if values, ok := q["actor_id"]; ok {
		if values[0] == "system" {
			f.SystemActor = true
		} else {
			id, err := validations.ParseUUID("actor_id", values[0])
			if err != nil {
				return f, err
			}
			f.ActorID = &id
		}
	}
	if values, ok := q["entity_id"]; ok {
		id, err := validations.ParseUUID("entity_id", values[0])
		if err != nil {
			return f, err
		}
		f.EntityID = &id
	}
	if _, ok := q["action"]; ok {
		v, err := validations.ParseEnum(
			q,
			"action",
			"",
			"create",
			"update",
			"password_change",
			"manual_writeoff",
			"expiration",
		)
		if err != nil {
			return f, err
		}
		f.Action = model.AuditAction(v)
	}
	if _, ok := q["entity_type"]; ok {
		v, err := validations.ParseEnum(
			q,
			"entity_type",
			"",
			"product_category",
			"product",
			"dish",
			"supply",
			"order",
			"stock_movement",
			"user",
			"settings",
		)
		if err != nil {
			return f, err
		}
		f.EntityType = model.AuditEntity(v)
	}
	for _, field := range []string{"from", "to"} {
		if values, ok := q[field]; ok {
			v, err := validations.ParseDateTime(field, values[0])
			if err != nil {
				return f, err
			}
			if field == "from" {
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
