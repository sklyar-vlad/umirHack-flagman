package user

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	Roles(context.Context) ([]dto.Role, error)
	List(context.Context, model.UserFilter) ([]dto.User, int64, error)
	Get(context.Context, uuid.UUID) (dto.User, error)
	Create(context.Context, dto.CreateUserRequest) (dto.User, error)
	Update(context.Context, uuid.UUID, dto.UpdateUserRequest) (dto.User, error)
	Password(context.Context, uuid.UUID, dto.UserPasswordRequest) (dto.User, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Roles(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	items, err := h.service.Roles(c.Request.Context())
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.RoleListResponse{Items: items})
}

func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры"})
		return
	}
	f, err := parseFilter(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	items, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(
		http.StatusOK,
		dto.UserListResponse{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize, Meta: map[string]any{}},
	)
}

func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	u, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

func parseFilter(query url.Values) (model.UserFilter, error) {
	if err := validations.ValidateQuery(query, "page", "page_size", "search", "role", "is_active", "sort"); err != nil {
		return model.UserFilter{}, err
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		return model.UserFilter{}, err
	}
	sort, err := validations.ParseSort(query, "name", "name")
	if err != nil {
		return model.UserFilter{}, err
	}
	active, err := validations.ParseEnum(query, "is_active", "all", "all", "true", "false")
	if err != nil {
		return model.UserFilter{}, err
	}
	role, err := validations.ParseEnum(query, "role", "all", "all", "admin", "manager", "storekeeper", "order_operator")
	if err != nil {
		return model.UserFilter{}, err
	}
	f := model.UserFilter{Page: page.Page, PageSize: page.PageSize, Descending: sort.Descending}
	if active != "all" {
		value := active == "true"
		f.IsActive = &value
	}
	if role != "all" {
		f.Role = &role
	}
	if values, ok := query["search"]; ok {
		f.Search = &values[0]
	}
	return f, nil
}
