package inventory

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	ListInventory(context.Context, model.InventoryFilter) (*dto.InventoryListResponse, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) ListInventory(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	filter, err := parseInventoryFilter(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.ListInventory(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseInventoryFilter(query url.Values) (model.InventoryFilter, error) {
	if err := validations.ValidateQuery(
		query,
		"page",
		"page_size",
		"search",
		"category_id",
		"filter",
		"sort",
	); err != nil {
		return model.InventoryFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.InventoryFilter{}, err
	}
	sort, err := validations.ParseSort(query, "name", "name", "nearest_expires_at")
	if err != nil {
		return model.InventoryFilter{}, err
	}
	status, err := validations.ParseEnum(
		query,
		"filter",
		"all",
		"all",
		"expiring",
		"expired",
		"out_of_stock",
		"deficit",
	)
	if err != nil {
		return model.InventoryFilter{}, err
	}
	filter := model.InventoryFilter{Page: page.Page, PageSize: page.PageSize, Sort: sort.Field, Filter: status}
	if values, exists := query["search"]; exists {
		filter.Search = &values[0]
	}
	if values, exists := query["category_id"]; exists {
		id, err := validations.ParseUUID("category_id", values[0])
		if err != nil {
			return model.InventoryFilter{}, err
		}
		filter.CategoryID = &id
	}
	return filter, nil
}
