package order

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const (
	roleAdmin           = "admin"
	roleManager         = "manager"
	roleOrderOperator   = "order_operator"
	queryField          = "query"
	invalidQueryMessage = "Некорректные параметры запроса"
	dateFromField       = "from"
	dateToField         = "to"
)

type Service interface {
	Check(context.Context, dto.OrderRequest) (dto.OrderCheck, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Check(c *gin.Context) {
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	switch actor.Role {
	case roleAdmin, roleManager, roleOrderOperator:
	default:
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: queryField, Message: invalidQueryMessage})
		return
	}
	if err := validations.ValidateQuery(query); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		basehandler.WriteError(c, apperrors.ErrInvalidJSON)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	request, err := decodeRequest(body)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	response, err := h.service.Check(c.Request.Context(), request)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}
