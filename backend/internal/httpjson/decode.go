package httpjson

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const bodyField = "body"

func DecodeObject(reader io.Reader, dst any) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	raw := bytes.TrimSpace(data)
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return apperrors.ErrInvalidJSON
	}
	if len(raw) == 0 || raw[0] != '{' {
		return &validations.FieldError{Field: bodyField, Message: "Ожидается JSON-объект"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return apperrors.ErrInvalidJSON
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return apperrors.ErrInvalidJSON
				}
				name := key.(string)
				if seen[name] || name != strings.ToLower(name) {
					return &validations.FieldError{
						Field:   bodyField,
						Message: "Повторяющиеся или некорректные имена полей",
					}
				}
				seen[name] = true
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case json.Delim('['):
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return nil
		}
	}
	if err := visit(); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return &validations.FieldError{Field: bodyField, Message: "Некорректные поля или типы"}
	}
	return nil
}
