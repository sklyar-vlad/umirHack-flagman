package dish

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const fieldRecipe = "recipe"

type Service interface {
	Create(context.Context, dto.CreateDishRequest) (dto.Dish, error)
	Update(context.Context, uuid.UUID, dto.UpdateDishRequest) (dto.Dish, error)
	Preview(context.Context, dto.PreviewDishRequest) (dto.DishPreview, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func invalid(field string) error {
	return &validations.FieldError{Field: field, Message: "Некорректные поля или типы"}
}

func object(data []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid("body")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, invalid("body")
		}
		name := key.(string)
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, invalid(name)
		}
		nullable, ok := allowed[name]
		if _, duplicate := fields[name]; !ok || duplicate ||
			(!nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return nil, invalid(name)
		}
		fields[name] = raw
	}
	return fields, nil
}

func decode(c *gin.Context, dst any, allowed map[string]bool) error {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) || !json.Valid(data) {
		return apperrors.ErrInvalidJSON
	}
	fields, err := object(data, allowed)
	if err != nil {
		return err
	}
	if raw, ok := fields[fieldRecipe]; ok {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return invalid(fieldRecipe)
		}
		for _, item := range items {
			if _, err := object(item, map[string]bool{"product_id": false, "quantity_per_portion": false}); err != nil {
				return err
			}
		}
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return invalid("body")
	}
	return nil
}

func (h *Handler) Create(c *gin.Context) {
	var input dto.CreateDishRequest
	if err := decode(
		c,
		&input,
		map[string]bool{"name": false, "description": true, "is_active": false, fieldRecipe: false},
	); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(201, result)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var input dto.UpdateDishRequest
	if err := decode(
		c,
		&input,
		map[string]bool{
			"name":              false,
			"description":       true,
			"is_active":         false,
			fieldRecipe:         false,
			"expected_revision": false,
		},
	); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Update(c.Request.Context(), id, input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}

func (h *Handler) Preview(c *gin.Context) {
	var input dto.PreviewDishRequest
	if err := decode(c, &input, map[string]bool{fieldRecipe: false}); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Preview(c.Request.Context(), input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}
