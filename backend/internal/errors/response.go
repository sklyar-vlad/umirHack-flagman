package errors

import (
	"errors"
	"net/http"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

// MakeResponse is shared by HTTP handlers and stored domain refusals.
func MakeResponse(
	err error,
	requestID string,
) (int, dto.ErrorResponse) {
	status := http.StatusInternalServerError

	body := dto.ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: "Внутренняя ошибка сервера",
		Details: map[string]any{},
	}

	var productInUse *ProductInUse
	var revisionConflict *RevisionConflict
	var fieldError *validations.FieldError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.Is(err, ErrBodyTooLarge) || errors.As(err, &maxBytesError):
		status = http.StatusRequestEntityTooLarge
		body.Code = "PAYLOAD_TOO_LARGE"
		body.Message = "Тело запроса превышает допустимый размер"
	case errors.Is(err, ErrTooManyRequests):
		status = http.StatusTooManyRequests
		body.Code = "TOO_MANY_REQUESTS"
		body.Message = "Слишком много запросов"
	case errors.Is(err, ErrInvalidJSON):
		status = http.StatusBadRequest
		body.Code = "INVALID_JSON"
		body.Message = "Некорректный JSON"
	case errors.As(err, &fieldError):
		status = http.StatusUnprocessableEntity
		body.Code = "VALIDATION_ERROR"
		body.Message = "Некорректные данные"
		if errors.Is(err, ErrShelfLifeRequired) {
			body.Code = "SHELF_LIFE_REQUIRED"
			body.Message = "Необходимо указать срок годности"
		}
		body.Details = map[string]any{
			"fields": []dto.FieldViolation{
				{
					Path:    fieldError.Field,
					Code:    "invalid_value",
					Message: fieldError.Message,
				},
			},
		}
	case errors.Is(err, ErrUnauthorized):
		status = http.StatusUnauthorized
		body.Code = "UNAUTHORIZED"
		body.Message = "Требуется авторизация"
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
		body.Code = "FORBIDDEN"
		body.Message = "Недостаточно прав"
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Ресурс не найден"
	case errors.Is(err, ErrIdempotencyConflict):
		status = http.StatusConflict
		body.Code = "IDEMPOTENCY_CONFLICT"
		body.Message = "Ключ уже использован с другим телом запроса"
	case errors.Is(err, ErrRequestInProgress):
		status = http.StatusConflict
		body.Code = "REQUEST_IN_PROGRESS"
		body.Message = "Запрос с этим ключом ещё выполняется"
	case errors.As(err, &revisionConflict):
		status = http.StatusConflict
		body.Code = "REVISION_CONFLICT"
		body.Message = "Ресурс уже изменён"
		body.Details = map[string]any{"current_revision": revisionConflict.CurrentRevision}
	case errors.Is(err, ErrLastAdmin):
		status = http.StatusConflict
		body.Code = "LAST_ADMIN_PROTECTED"
		body.Message = "Нельзя отключить или понизить последнего активного администратора"
	case errors.Is(err, ErrUnitLocked):
		status = http.StatusConflict
		body.Code = "UNIT_LOCKED"
		body.Message = "Единица продукта уже используется в истории"
	case errors.As(err, &productInUse):
		status = http.StatusConflict
		body.Code = "PRODUCT_IN_USE"
		body.Message = "Продукт используется активными блюдами"
		body.Details = map[string]any{"dishes": productInUse.Dishes}
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
		body.Code = "CONFLICT"
		body.Message = "Конфликт состояния ресурса"
	case errors.Is(err, ErrUnavailable):
		status = http.StatusServiceUnavailable
		body.Code = "SERVICE_UNAVAILABLE"
		body.Message = "Сервис временно недоступен"
	}

	return status, dto.ErrorResponse{
		Error:     body,
		RequestID: requestID,
	}
}
