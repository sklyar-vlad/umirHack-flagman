package dashboard

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	Get(context.Context, string) (dto.DashboardResponse, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Get(c *gin.Context) {
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	switch actor.Role {
	case "admin", "manager", "storekeeper", "order_operator":
	default:
		basehandler.WriteError(c, apperrors.ErrForbidden)
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
	result, err := h.service.Get(c.Request.Context(), actor.Role)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
