package product

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
	ListProducts(context.Context, model.ProductFilter) ([]dto.Product, int64, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) ListProducts(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	filter, err := parseProductFilter(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.ListProducts(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ProductListResponse{
		Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize, Meta: map[string]any{},
	})
}

func parseProductFilter(query url.Values) (model.ProductFilter, error) {
	if err := validations.ValidateQuery(
		query,
		"page",
		"page_size",
		"search",
		"category_id",
		"is_active",
		"sort",
	); err != nil {
		return model.ProductFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.ProductFilter{}, err
	}
	if _, err := validations.ParseSort(query, "name", "name"); err != nil {
		return model.ProductFilter{}, err
	}
	active, err := validations.ParseEnum(query, "is_active", "true", "all", "true", "false")
	if err != nil {
		return model.ProductFilter{}, err
	}
	filter := model.ProductFilter{Page: page.Page, PageSize: page.PageSize}
	if active != "all" {
		value := active == "true"
		filter.IsActive = &value
	}
	if values, exists := query["search"]; exists {
		filter.Search = &values[0]
	}
	if values, exists := query["category_id"]; exists {
		id, err := validations.ParseUUID("category_id", values[0])
		if err != nil {
			return model.ProductFilter{}, err
		}
		filter.CategoryID = &id
	}
	return filter, nil
}
