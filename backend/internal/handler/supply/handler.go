package supply

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	ListSupplies(context.Context, model.SupplyFilter) ([]supplydto.SupplySummaryDTO, int64, error)
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
	query, err := parseSupplyQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
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
	query, err := parseSupplyQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if err := validations.ValidateQuery(query); err != nil {
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

// GET /api/v1/supplies
func (h *Handler) ListSupplies(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	query, err := parseSupplyQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	filter, err := parseSupplyFilter(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}

	items, total, err := h.service.ListSupplies(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
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
