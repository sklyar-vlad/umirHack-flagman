package order

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type HistoryService interface {
	ListOrders(context.Context, model.OrderHistoryFilter) ([]dto.OrderSummary, int64, error)
	GetOrder(context.Context, uuid.UUID) (dto.OrderDetail, error)
	ListAttempts(context.Context, model.OrderHistoryFilter) ([]dto.OrderAttempt, int64, error)
}

type HistoryHandler struct{ service HistoryService }

func NewHistoryHandler(service HistoryService) *HistoryHandler {
	return &HistoryHandler{service: service}
}

func requireHistoryActor(c *gin.Context) bool {
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return false
	}
	switch actor.Role {
	case "admin", "manager", "order_operator":
		return true
	default:
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return false
	}
}

func (h *HistoryHandler) ListOrders(c *gin.Context) {
	if !requireHistoryActor(c) {
		return
	}
	filter, err := parseHistoryFilter(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.ListOrders(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ListResponse[dto.OrderSummary]{Items: items, Total: total,
		Page: filter.Page, PageSize: filter.PageSize, Meta: map[string]any{}})
}

func (h *HistoryHandler) GetOrder(c *gin.Context) {
	if !requireHistoryActor(c) {
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	if err := validations.ValidateQuery(query); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.GetOrder(c.Request.Context(), id)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseHistoryFilter(raw string) (model.OrderHistoryFilter, error) {
	return parseHistoryQuery(raw, false)
}

func (h *HistoryHandler) ListAttempts(c *gin.Context) {
	if !requireHistoryActor(c) {
		return
	}
	filter, err := parseHistoryQuery(c.Request.URL.RawQuery, true)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.ListAttempts(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ListResponse[dto.OrderAttempt]{Items: items, Total: total,
		Page: filter.Page, PageSize: filter.PageSize, Meta: map[string]any{}})
}

func parseHistoryQuery(raw string, attempts bool) (model.OrderHistoryFilter, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return model.OrderHistoryFilter{}, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"}
	}
	allowed := []string{"page", "page_size", "search", "from", "to", "sort"}
	if attempts {
		allowed = append(allowed, "reason_code")
	}
	if err := validations.ValidateQuery(query, allowed...); err != nil {
		return model.OrderHistoryFilter{}, err
	}
	pagination, err := validations.ParsePagination(query)
	if err != nil {
		return model.OrderHistoryFilter{}, err
	}
	if _, err := validations.ParseSort(query, "-created_at", "-created_at"); err != nil {
		return model.OrderHistoryFilter{}, err
	}
	filter := model.OrderHistoryFilter{Page: pagination.Page, PageSize: pagination.PageSize, Search: query.Get("search")}
	if _, exists := query["reason_code"]; attempts && exists {
		filter.ReasonCode, err = validations.ParseEnum(query, "reason_code", "", "INSUFFICIENT_STOCK", "RECIPE_CHANGED", "DISH_UNAVAILABLE")
		if err != nil {
			return model.OrderHistoryFilter{}, err
		}
	}
	for _, field := range []string{"from", "to"} {
		if values, exists := query[field]; exists {
			value, err := validations.ParseDateTime(field, values[0])
			if err != nil {
				return model.OrderHistoryFilter{}, err
			}
			if field == "from" {
				filter.DateFrom = &value
			} else {
				filter.DateTo = &value
			}
		}
	}
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateFrom.After(*filter.DateTo) {
		return model.OrderHistoryFilter{}, &validations.FieldError{Field: "to", Message: "Конец интервала не может быть раньше начала"}
	}
	return filter, nil
}
