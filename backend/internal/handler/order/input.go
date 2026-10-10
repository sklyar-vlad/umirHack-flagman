package order

import (
	"bytes"
	"encoding/json"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

// decodeRequest distinguishes malformed JSON from a valid JSON value with an
// invalid basket shape. Numeric portions and versions must be JSON integers.
func decodeRequest(body []byte) (dto.OrderRequest, error) {
	if !json.Valid(body) {
		return dto.OrderRequest{}, apperrors.ErrInvalidJSON
	}
	trimmed := bytes.TrimSpace(body)
	if trimmed[0] != '{' {
		return dto.OrderRequest{}, &validations.FieldError{Field: "body", Message: "Ожидается объект корзины"}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request dto.OrderRequest
	if err := decoder.Decode(&request); err != nil {
		return dto.OrderRequest{}, &validations.FieldError{Field: "body", Message: "Некорректная структура корзины: ожидаются items, dish_id, целые portions и recipe_version"}
	}
	return request, nil
}
