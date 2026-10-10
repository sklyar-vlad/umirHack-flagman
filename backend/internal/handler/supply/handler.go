package supply

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	ListSupplies(context.Context, model.SupplyFilter) ([]supplydto.SupplySummaryDTO, int64, error)
	GetSupplyByID(context.Context, uuid.UUID, time.Time) (*supplydto.SupplyDetailDTO, error)
}

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// GET /api/v1/supplies
func (h *Handler) ListSupplies(c *gin.Context) {
	query, err := parseSupplyQuery(c.Request.URL.RawQuery)
	if err != nil {
		handler.WriteError(c, err)
		return
	}
	filter, err := parseSupplyFilter(query)
	if err != nil {
		handler.WriteError(c, err)
		return
	}

	items, total, err := h.svc.ListSupplies(c.Request.Context(), filter)
	if err != nil {
		handler.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, supplydto.SupplyListResponse{
		Items:    items,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Meta:     map[string]any{},
	})
}

// GET /api/v1/supplies/:id
func (h *Handler) GetSupply(c *gin.Context) {
	query, err := parseSupplyQuery(c.Request.URL.RawQuery)
	if err != nil {
		handler.WriteError(c, err)
		return
	}
	if err := validations.ValidateQuery(query); err != nil {
		handler.WriteError(c, err)
		return
	}
	idParam := c.Param("id")
	supplyID, err := validations.ParseUUID("id", idParam)
	if err != nil {
		handler.WriteError(c, err)
		return
	}

	now := time.Now().UTC()
	supplyDetail, err := h.svc.GetSupplyByID(c.Request.Context(), supplyID, now)
	if err != nil {
		handler.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, supplyDetail)
}

func parseSupplyQuery(raw string) (url.Values, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"}
	}
	return query, nil
}

func parseSupplyFilter(query url.Values) (model.SupplyFilter, error) {
	if err := validations.ValidateQuery(query, "page", "page_size", "search", "from", "to", "sort"); err != nil {
		return model.SupplyFilter{}, err
	}
	pagination, err := validations.ParsePagination(query)
	if err != nil {
		return model.SupplyFilter{}, err
	}
	if _, err := validations.ParseSort(query, "-created_at", "-created_at"); err != nil {
		return model.SupplyFilter{}, err
	}
	filter := model.SupplyFilter{Page: pagination.Page, PageSize: pagination.PageSize}
	if values, exists := query["search"]; exists {
		filter.Search = &values[0]
	}
	for _, field := range []string{"from", "to"} {
		if values, exists := query[field]; exists {
			value, err := validations.ParseDateTime(field, values[0])
			if err != nil {
				return model.SupplyFilter{}, err
			}
			if field == "from" {
				filter.DateFrom = &value
			} else {
				filter.DateTo = &value
			}
		}
	}
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateFrom.After(*filter.DateTo) {
		return model.SupplyFilter{}, &validations.FieldError{
			Field:   "to",
			Message: "Конец интервала не может быть раньше начала",
		}
	}
	return filter, nil
}
