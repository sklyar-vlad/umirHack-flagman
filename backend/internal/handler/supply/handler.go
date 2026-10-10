package supply

import (
	"context"
	"io"
	"mime"
	"net/http"

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
	CreateSupplyIdempotent(context.Context, uuid.UUID, uuid.UUID, []byte) (model.IdempotencyResponse, error)
	GetSupply(context.Context, uuid.UUID) (dto.SupplyDetail, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) CreateSupply(c *gin.Context) {
	actor, ok := requestctx.Actor(c.Request.Context())
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	if err := validations.ValidateQuery(c.Request.URL.Query()); err != nil {
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
	response, err := h.service.CreateSupplyIdempotent(c.Request.Context(), actor.ID, key, body)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	basehandler.WriteIdempotencyResponse(c, &response)
}

func (h *Handler) GetSupply(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	if err := validations.ValidateQuery(c.Request.URL.Query()); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	details, err := h.service.GetSupply(c.Request.Context(), id)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, details)
}
