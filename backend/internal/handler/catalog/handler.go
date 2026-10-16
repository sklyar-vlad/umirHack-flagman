package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	ListCategories(context.Context, model.CategoryFilter) ([]dto.CatalogCategory, int64, error)
	GetCategory(context.Context, uuid.UUID) (dto.CatalogCategory, error)
	CreateCategory(context.Context, dto.CreateCategoryRequest) (dto.CatalogCategory, error)
	UpdateCategory(context.Context, uuid.UUID, dto.UpdateCategoryRequest) (dto.CatalogCategory, error)
	GetProduct(context.Context, uuid.UUID) (dto.Product, error)
	CreateProduct(context.Context, *dto.CreateProductRequest) (dto.Product, error)
	UpdateProduct(context.Context, uuid.UUID, *dto.UpdateProductRequest) (dto.Product, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func decode(c *gin.Context, dst any, product bool) error {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) || !json.Valid(data) {
		return apperrors.ErrInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, _ := decoder.Token()
	if token != json.Delim('{') {
		return &validations.FieldError{Field: "body", Message: "Ожидается объект"}
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, _ := decoder.Token()
		name := key.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return apperrors.ErrInvalidJSON
		}
		nullable := product && (name == "target_stock" || name == "standard_shelf_life_hours")
		if name != strings.ToLower(name) || seen[name] ||
			(!nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return &validations.FieldError{Field: name, Message: "Повтор или null недопустим"}
		}
		seen[name] = true
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return &validations.FieldError{Field: "body", Message: "Некорректные поля или типы"}
	}
	return nil
}

func respond(c *gin.Context, status int, value any, err error) {
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(status, value)
}

func (h *Handler) ListCategories(c *gin.Context) {
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры"})
		return
	}
	if err := validations.ValidateQuery(query, "page", "page_size", "search", "sort"); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	page, err := validations.ParsePagination(query)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if _, err := validations.ParseSort(query, "name", "name"); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	f := model.CategoryFilter{Page: page.Page, PageSize: page.PageSize}
	if values, ok := query["search"]; ok {
		f.Search = &values[0]
	}
	items, total, err := h.service.ListCategories(c.Request.Context(), f)
	respond(
		c,
		200,
		dto.CategoryListResponse{
			Items:    items,
			Total:    total,
			Page:     f.Page,
			PageSize: f.PageSize,
			Meta:     map[string]any{},
		},
		err,
	)
}

func (h *Handler) GetCategory(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.GetCategory(c.Request.Context(), id)
	respond(c, 200, value, err)
}

func (h *Handler) GetProduct(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.GetProduct(c.Request.Context(), id)
	respond(c, 200, value, err)
}

func (h *Handler) CreateCategory(c *gin.Context) {
	var input dto.CreateCategoryRequest
	if err := decode(c, &input, false); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.CreateCategory(c.Request.Context(), input)
	respond(c, http.StatusCreated, value, err)
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var input dto.UpdateCategoryRequest
	if err := decode(c, &input, false); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.UpdateCategory(c.Request.Context(), id, input)
	respond(c, 200, value, err)
}

func (h *Handler) CreateProduct(c *gin.Context) {
	var input dto.CreateProductRequest
	if err := decode(c, &input, true); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.CreateProduct(c.Request.Context(), &input)
	respond(c, http.StatusCreated, value, err)
}

func (h *Handler) UpdateProduct(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var input dto.UpdateProductRequest
	if err := decode(c, &input, true); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	value, err := h.service.UpdateProduct(c.Request.Context(), id, &input)
	respond(c, 200, value, err)
}
