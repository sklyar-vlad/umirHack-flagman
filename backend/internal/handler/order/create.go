package order

import (
	"context"
	"io"
	"mime"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type CreateService interface {
	CreateIdempotent(context.Context, uuid.UUID, uuid.UUID, []byte) (model.IdempotencyResponse, error)
}

type CreateHandler struct{ service CreateService }

func NewCreateHandler(service CreateService) *CreateHandler { return &CreateHandler{service: service} }

func (h *CreateHandler) Create(c *gin.Context) {
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
	if err := validations.ValidateQuery(query); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	key, err := basehandler.ReadIdempotencyKey(c)
	if err != nil {
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
	response, err := h.service.CreateIdempotent(c.Request.Context(), actor.ID, key, body)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	basehandler.WriteIdempotencyResponse(c, &response)
}
