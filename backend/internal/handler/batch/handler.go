package batch

import (
	"context"
	"net/http"
	"net/url"

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
	ListBatches(context.Context, model.BatchFilter) (*dto.BatchListResponse, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) ListBatches(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры запроса"})
		return
	}
	filter, err := parseBatchFilter(id, query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.ListBatches(c.Request.Context(), filter)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseBatchFilter(id uuid.UUID, query url.Values) (model.BatchFilter, error) {
	if err := validations.ValidateQuery(query, "page", "page_size", "status", "supply_id", "sort"); err != nil {
		return model.BatchFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.BatchFilter{}, err
	}
	sort, err := validations.ParseSort(query, "expires_at", "expires_at", "-created_at")
	if err != nil {
		return model.BatchFilter{}, err
	}
	filter := model.BatchFilter{ProductID: id, Page: page.Page, PageSize: page.PageSize, Sort: sort.Field}
	if sort.Descending {
		filter.Sort = "-" + sort.Field
	}
	if _, exists := query["status"]; exists {
		filter.Status, err = validations.ParseEnum(
			query,
			"status",
			"all",
			"all",
			"valid",
			"expiring",
			"expired",
			"depleted",
		)
		if err != nil {
			return model.BatchFilter{}, err
		}
	}
	if values, exists := query["supply_id"]; exists {
		id, err := validations.ParseUUID("supply_id", values[0])
		if err != nil {
			return model.BatchFilter{}, err
		}
		filter.SupplyID = &id
	}
	return filter, nil
}
