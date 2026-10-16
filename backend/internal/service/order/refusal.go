package order

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
)

func (s *CreateService) saveRefusal(
	ctx context.Context,
	tx repository.DBTX,
	actor uuid.UUID,
	check *dto.OrderCheck,
	now time.Time,
) (idempotency.Outcome, error) {
	code, message := refusalReason(check)
	attempt := model.OrderAttempt{ID: uuid.New(), ActorID: actor, CreatedAt: now, ReasonCode: code}
	var err error
	attempt.ItemsSnapshot, err = json.Marshal(check.Items)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	attempt.Shortages, err = json.Marshal(check.Shortages)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	attempt.Issues, err = json.Marshal(check.Issues)
	if err != nil {
		return idempotency.Outcome{}, err
	}
	if err := s.repo.InsertAttempt(ctx, tx, &attempt); err != nil {
		return idempotency.Outcome{}, err
	}
	body, err := json.Marshal(
		dto.ErrorResponse{
			RequestID: requestctx.ID(ctx),
			Error: dto.ErrorBody{Code: code, Message: message, Details: map[string]any{
				"attempt_id":   attempt.ID,
				"requirements": check.Requirements,
				"shortages":    check.Shortages,
				"issues":       check.Issues,
			}},
		},
	)
	if err != nil {
		return idempotency.Outcome{}, err
	}

	return idempotency.Outcome{Response: model.IdempotencyResponse{Status: http.StatusConflict, Body: body}}, nil
}

func refusalReason(check *dto.OrderCheck) (string, string) {
	for _, issue := range check.Issues {
		if issue.Code == "recipe_changed" {
			return "RECIPE_CHANGED", "Рецептура блюда изменилась"
		}
	}
	if len(check.Issues) > 0 {
		return "DISH_UNAVAILABLE", "Блюдо недоступно для заказа"
	}
	return "INSUFFICIENT_STOCK", "Недостаточно ингредиентов для заказа"
}
