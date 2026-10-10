package handler

import (
	"fmt"
	"net/http"
	"testing"

	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestShelfLifeRequiredResponse(t *testing.T) {
	err := fmt.Errorf("%w: %w", apperrors.ErrShelfLifeRequired,
		&validations.FieldError{Field: "items[0].shelf_life_hours", Message: "required"})
	status, response := makeErrorResponse(err, "test")
	if status != http.StatusUnprocessableEntity || response.Error.Code != "SHELF_LIFE_REQUIRED" {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}
