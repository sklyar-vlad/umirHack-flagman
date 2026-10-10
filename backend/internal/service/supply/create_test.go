package supply

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
)

func TestCreateRequiresActorBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil, nil, nil)
	result, err := s.Create(context.Background(), uuid.Nil, dto.CreateSupplyRequest{})
	if !errors.Is(err, apperrors.ErrUnauthorized) || result.ID != uuid.Nil {
		t.Fatalf("expected unauthorized and empty result, got %+v, %v", result, err)
	}
}
