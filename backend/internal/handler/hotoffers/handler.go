package hotoffers

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
	List(context.Context, model.HotOffersFilter) (dto.HotOffersListResponse, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *gin.Context) {
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	switch actor.Role {
	case "admin", "manager", "order_operator":
	default:
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	filter, err := parseFilter(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseFilter(query url.Values) (model.HotOffersFilter, error) {
	if err := validations.ValidateQuery(query, "page", "page_size", "search"); err != nil {
		return model.HotOffersFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.HotOffersFilter{}, err
	}
	filter := model.HotOffersFilter{Page: page.Page, PageSize: page.PageSize}
	if search, ok := query["search"]; ok {
		filter.Search = &search[0]
	}
	return filter, nil
}
