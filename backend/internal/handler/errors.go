package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

var (
	errInvalidJSON     = errors.New("invalid JSON")
	errBodyTooLarge    = errors.New("request body too large")
	errTooManyRequests = errors.New("too many requests")
)

func makeErrorResponse(
	err error,
	requestID string,
) (int, dto.ErrorResponse) {
	status := http.StatusInternalServerError

	body := dto.ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: "Внутренняя ошибка сервера",
		Details: map[string]any{},
	}

	var fieldError *validations.FieldError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.Is(err, errBodyTooLarge) || errors.As(err, &maxBytesError):
		status = http.StatusRequestEntityTooLarge
		body.Code = "PAYLOAD_TOO_LARGE"
		body.Message = "Тело запроса превышает допустимый размер"
	case errors.Is(err, errTooManyRequests):
		status = http.StatusTooManyRequests
		body.Code = "TOO_MANY_REQUESTS"
		body.Message = "Слишком много запросов"
	case errors.Is(err, errInvalidJSON):
		status = http.StatusBadRequest
		body.Code = "INVALID_JSON"
		body.Message = "Некорректный JSON"
	case errors.As(err, &fieldError):
		status = http.StatusUnprocessableEntity
		body.Code = "VALIDATION_ERROR"
		body.Message = "Некорректные данные"
		body.Details = map[string]any{
			"fields": []dto.FieldViolation{
				{
					Path:    fieldError.Field,
					Code:    "invalid_value",
					Message: fieldError.Message,
				},
			},
		}
	case errors.Is(err, apperrors.ErrUnauthorized):
		status = http.StatusUnauthorized
		body.Code = "UNAUTHORIZED"
		body.Message = "Требуется авторизация"
	case errors.Is(err, apperrors.ErrForbidden):
		status = http.StatusForbidden
		body.Code = "FORBIDDEN"
		body.Message = "Недостаточно прав"
	case errors.Is(err, apperrors.ErrNotFound):
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Ресурс не найден"
	case errors.Is(err, apperrors.ErrConflict):
		status = http.StatusConflict
		body.Code = "CONFLICT"
		body.Message = "Конфликт состояния ресурса"
	case errors.Is(err, apperrors.ErrUnavailable):
		status = http.StatusServiceUnavailable
		body.Code = "SERVICE_UNAVAILABLE"
		body.Message = "Сервис временно недоступен"
	}

	return status, dto.ErrorResponse{
		Error:     body,
		RequestID: requestID,
	}
}

func writeError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	_ = c.Error(err)

	if c.Writer.Written() {
		c.Abort()
		return
	}

	status, response := makeErrorResponse(
		err,
		requestctx.ID(c.Request.Context()),
	)

	c.AbortWithStatusJSON(status, response)
}

func NotFound(c *gin.Context) {
	writeError(c, apperrors.ErrNotFound)
}
