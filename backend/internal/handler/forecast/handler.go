package forecast

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

type Service interface {
	List(context.Context, model.ForecastFilter) (dto.ListResponse[dto.StopListForecast], error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseFilter(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}
