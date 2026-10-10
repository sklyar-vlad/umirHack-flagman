package supply

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service"
)

type Handler struct {
	svc service.Service
}

func NewHandler(svc service.Service) *Handler {
	return &Handler{svc: svc}
}


// GET /api/v1/supplies
func (h *Handler) ListSupplies(c *gin.Context) {
	// 1. Валидация пагинации и query params через твои validations
	page := 1
	pageSize := 50
	// (разбор query params и проверка на единичность параметров согласно S02)

	filter := model.SupplyFilter{
		Page:     page,
		PageSize: pageSize,
	}

	if s := c.Query("search"); s != "" {
		filter.Search = &s
	}

	items, total, err := h.svc.ListSupplies(c.Request.Context(), filter)
	if err != nil {
		handler.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ListEnvelope[any]{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Meta:     dto.Meta{},
	})
}

// GET /api/v1/supplies/:id
func (h *Handler) GetSupply(c *gin.Context) {
	idParam := c.Param("id")
	supplyID, err := uuid.Parse(idParam)
	if err != nil {
		handler.HandleValidationError(c, "id", "invalid_uuid", "Некорректный UUID")
		return
	}

	now := time.Now().UTC()
	supplyDetail, err := h.svc.GetSupplyByID(c.Request.Context(), supplyID, now)
	if err != nil {
		handler.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, supplyDetail)
}