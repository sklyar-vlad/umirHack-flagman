package order

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestDecodeRequest(t *testing.T) {
	id := uuid.NewString()
	valid := `{"items":[{"dish_id":"` + id + `","portions":2,"recipe_version":1}]}`
	request, err := decodeRequest([]byte(valid))
	if err != nil || len(request.Items) != 1 || request.Items[0].DishID != id || request.Items[0].Portions != 2 || *request.Items[0].RecipeVersion != 1 {
		t.Fatalf("decode valid request: %+v %v", request, err)
	}
	for _, raw := range []string{"", "{", valid + `{}`, `{"items":`} {
		if _, err := decodeRequest([]byte(raw)); !errors.Is(err, apperrors.ErrInvalidJSON) {
			t.Errorf("malformed JSON %q: %v", raw, err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `true`, `{"items":{}}`, `{"unknown":1}`, `{"items":[{"dish_id":1}]}`} {
		var fieldError *validations.FieldError
		if _, err := decodeRequest([]byte(raw)); !errors.As(err, &fieldError) {
			t.Errorf("invalid shape %q: %v", raw, err)
		}
	}
	for _, field := range []string{"portions", "recipe_version"} {
		for _, value := range []string{`1.5`, `1.0`, `"2"`, `true`, `9223372036854775808`} {
			raw := `{"items":[{"dish_id":"` + id + `","` + field + `":` + value + `}]}`
			var fieldError *validations.FieldError
			if _, err := decodeRequest([]byte(raw)); !errors.As(err, &fieldError) {
				t.Errorf("invalid %s %s: %v", field, value, err)
			}
		}
	}
}
