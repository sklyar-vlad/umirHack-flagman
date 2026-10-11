package user

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

// Validate shape before decoding: nulls, repeated keys and unknown fields must
// never silently erase fields or override a concurrency version.
func decode(c *gin.Context, destination any) error {
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
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return apperrors.ErrInvalidJSON
		}
		if seen[name] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return &validations.FieldError{Field: name, Message: "Повтор или null недопустим"}
		}
		seen[name] = true
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return &validations.FieldError{Field: "body", Message: "Некорректные поля или типы"}
	}
	return nil
}

func (h *Handler) Create(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input dto.CreateUserRequest
	if err := decode(c, &input); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	u, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, u)
}

func (h *Handler) Update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var input dto.UpdateUserRequest
	if err := decode(c, &input); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	u, err := h.service.Update(c.Request.Context(), id, input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

func (h *Handler) Password(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var input dto.UserPasswordRequest
	if err := decode(c, &input); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	u, err := h.service.Password(c.Request.Context(), id, input)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}
