package writeoff

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

type Service interface {
	CreateIdempotent(context.Context, uuid.UUID, uuid.UUID, []byte) (model.IdempotencyResponse, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Create(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	if actor.Role != "admin" && actor.Role != "manager" && actor.Role != "storekeeper" {
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return
	}
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры"})
		return
	}
	if err = validations.ValidateQuery(q); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	key, err := basehandler.ReadIdempotencyKey(c)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
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
