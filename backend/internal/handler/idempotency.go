package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func ReadIdempotencyKey(c *gin.Context) (uuid.UUID, error) {
	values := c.Request.Header.Values("Idempotency-Key")
	if len(values) != 1 {
		return uuid.Nil, &validations.FieldError{Field: "Idempotency-Key", Message: "Необходим ровно один UUID ключа"}
	}
	key, err := validations.ParseUUID("Idempotency-Key", values[0])
	if err != nil {
		return uuid.Nil, err
	}
	if key == uuid.Nil {
		return uuid.Nil, &validations.FieldError{Field: "Idempotency-Key", Message: "Нулевой UUID ключа недопустим"}
	}
	return key, nil
}

func WriteIdempotencyResponse(c *gin.Context, response *model.IdempotencyResponse) {
	if response.Location != "" {
		c.Header("Location", response.Location)
	}
	c.Data(response.Status, "application/json; charset=utf-8", response.Body)
}
