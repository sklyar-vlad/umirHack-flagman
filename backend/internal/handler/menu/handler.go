package menu

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

type Service interface {
	ListDishes(context.Context, *model.DishFilter) (dto.DishListResponse, error)
	GetDish(context.Context, uuid.UUID) (dto.Dish, error)
	StopList(context.Context, *model.DishFilter) (dto.DishListResponse, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func authorize(c *gin.Context) bool {
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

func (h *Handler) ListDishes(c *gin.Context) { h.list(c, false) }
func (h *Handler) StopList(c *gin.Context)   { h.list(c, true) }

func (h *Handler) list(c *gin.Context, stopList bool) {
	if !authorize(c) {
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	filter, err := parseFilter(query, stopList)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var response dto.DishListResponse
	if stopList {
		response, err = h.service.StopList(c.Request.Context(), &filter)
	} else {
		response, err = h.service.ListDishes(c.Request.Context(), &filter)
	}
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetDish(c *gin.Context) {
	if !authorize(c) {
		return
	}
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		err = validations.ValidateQuery(query)
	}
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	dish, err := h.service.GetDish(c.Request.Context(), id)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, dish)
}

func parseFilter(query url.Values, stopList bool) (model.DishFilter, error) {
	allowed := []string{"page", "page_size", "search", "sort"}
	if !stopList {
		allowed = append(allowed, "is_active", "availability")
	}
	if err := validations.ValidateQuery(query, allowed...); err != nil {
		return model.DishFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.DishFilter{}, err
	}
	if _, err := validations.ParseSort(query, "name", "name"); err != nil {
		return model.DishFilter{}, err
	}
	filter := model.DishFilter{Page: page.Page, PageSize: page.PageSize, Availability: "all"}
	if search, ok := query["search"]; ok {
		filter.Search = &search[0]
	}
	if stopList {
		return filter, nil
	}
	active, err := validations.ParseEnum(query, "is_active", "true", "all", "true", "false")
	if err != nil {
		return model.DishFilter{}, err
	}
	if active != "all" {
		value := active == "true"
		filter.IsActive = &value
	}
	filter.Availability, err = validations.ParseEnum(query, "availability", "all", "all", "available", "unavailable")
	return filter, err
}
