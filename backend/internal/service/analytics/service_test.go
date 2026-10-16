package analytics

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

func TestAnalyticsServiceRejectsMissingAndForbiddenActorsBeforeDatabase(t *testing.T) {
	s := NewService(nil, nil)
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{
		{context.Background(), apperrors.ErrUnauthorized},
		{requestctx.WithActor(context.Background(), model.Actor{Role: "admin"}), apperrors.ErrUnauthorized},
		{requestctx.WithActor(context.Background(), model.Actor{ID: uuid.New(), Role: "storekeeper"}), apperrors.ErrForbidden},
		{requestctx.WithActor(context.Background(), model.Actor{ID: uuid.New(), Role: "order_operator"}), apperrors.ErrForbidden},
	} {
		if _, _, err := s.ProductFlow(tc.ctx, model.AnalyticsFilter{}); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
		if _, _, err := s.WriteOffs(tc.ctx, model.AnalyticsFilter{}); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
		if _, _, err := s.IngredientShortages(tc.ctx, model.AnalyticsFilter{}); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}

func TestAnalyticsErrorsPreserveCancellationAndHidePostgresDetails(t *testing.T) {
	original := fmt.Errorf(
		"wrapped: %w",
		&pgconn.PgError{Code: "22003", Message: "secret-message", Detail: "secret-detail"},
	)
	if got := safeError(original).Error(); got != "analytics: database error (SQLSTATE 22003)" {
		t.Fatal(got)
	}
	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		if safeError(err) != err {
			t.Fatal(err)
		}
	}
}
