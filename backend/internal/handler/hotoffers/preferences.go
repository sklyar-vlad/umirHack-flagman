package hotoffers

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/httpjson"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type PreferenceService interface {
	Get(context.Context, uuid.UUID) (model.HotOfferPreference, error)
	Save(context.Context, uuid.UUID, dto.UpdateHotOffer) (model.HotOfferPreference, error)
}
type PreferenceHandler struct{ service PreferenceService }

func NewPreferenceHandler(s PreferenceService) *PreferenceHandler {
	return &PreferenceHandler{service: s}
}

func (h *PreferenceHandler) Update(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var body dto.UpdateHotOffer
	if err = httpjson.DecodeObject(c.Request.Body, &body); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Save(c.Request.Context(), id, body)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}

func (h *PreferenceHandler) Get(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, result)
}
