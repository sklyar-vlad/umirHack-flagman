package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	Get(context.Context) (dto.Settings, error)
	Update(context.Context, dto.UpdateSettingsRequest) (dto.Settings, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "query", Message: "Некорректные параметры"})
		return
	}
	if err := validations.ValidateQuery(q); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Get(c.Request.Context())
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}
func decode(c *gin.Context, input *dto.UpdateSettingsRequest) error {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) || !json.Valid(data) {
		return apperrors.ErrInvalidJSON
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, _ := d.Token()
	if token != json.Delim('{') {
		return &validations.FieldError{Field: "body", Message: "Ожидается объект"}
	}
	allowed := map[string]bool{"restaurant_name": true, "timezone": true, "expiring_threshold_hours": true, "auto_writeoff_expired": true, "expected_revision": true}
	seen := map[string]bool{}
	for d.More() {
		key, _ := d.Token()
		name := key.(string)
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return apperrors.ErrInvalidJSON
		}
		if !allowed[name] || seen[name] || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return &validations.FieldError{Field: name, Message: "Неизвестное поле, повтор или null"}
		}
		seen[name] = true
	}
	if err := json.Unmarshal(data, input); err != nil {
		return &validations.FieldError{Field: "body", Message: "Некорректные типы"}
	}
	return nil
}
func (h *Handler) Update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input dto.UpdateSettingsRequest
	if err := decode(c, &input); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Update(c.Request.Context(), input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(200, result)
}
