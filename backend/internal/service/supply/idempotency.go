package supply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
)

type ResponseRenderer func(context.Context, repository.DBTX, *model.CreatedSupply) (json.RawMessage, error)

func (s *Service) CreateIdempotent(
	ctx context.Context,
	actorID uuid.UUID,
	key uuid.UUID,
	body []byte,
	render ResponseRenderer,
) (model.IdempotencyResponse, error) {
	if actorID == uuid.Nil {
		return model.IdempotencyResponse{}, apperrors.ErrUnauthorized
	}
	if render == nil {
		return model.IdempotencyResponse{}, fmt.Errorf("supply response renderer is required")
	}
	input, err := decodeSupplyBody(body)
	if err != nil {
		return model.IdempotencyResponse{}, err
	}
	if _, err := prepareSupplyInput(input); err != nil {
		return model.IdempotencyResponse{}, err
	}
	scope := model.IdempotencyScope{UserID: actorID, Method: http.MethodPost, Path: "/api/v1/supplies", Key: key}
	return s.idempotency.Execute(ctx, scope, body, func(tx repository.DBTX) (idempotency.Outcome, error) {
		created, err := s.createInTx(ctx, tx, actorID, input)
		if err != nil {
			return supplyRefusal(ctx, err)
		}
		responseBody, err := render(ctx, tx, &created)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		return idempotency.Outcome{Response: model.IdempotencyResponse{
			Status: http.StatusCreated, Body: responseBody, Location: "/api/v1/supplies/" + created.ID.String(),
		}}, nil
	})
}

func decodeSupplyBody(body []byte) (dto.CreateSupplyRequest, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return dto.CreateSupplyRequest{}, apperrors.ErrInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var input dto.CreateSupplyRequest
	if err := decoder.Decode(&input); err != nil {
		return dto.CreateSupplyRequest{}, apperrors.ErrInvalidJSON
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return dto.CreateSupplyRequest{}, apperrors.ErrInvalidJSON
	}
	return input, nil
}

func supplyRefusal(ctx context.Context, err error) (idempotency.Outcome, error) {
	status, response := apperrors.MakeResponse(err, requestctx.ID(ctx))
	if status >= http.StatusInternalServerError {
		return idempotency.Outcome{}, err
	}
	body, encodeErr := json.Marshal(response)
	if encodeErr != nil {
		return idempotency.Outcome{}, fmt.Errorf("encode supply refusal: %w", encodeErr)
	}
	return idempotency.Outcome{Response: model.IdempotencyResponse{Status: status, Body: body}, Rollback: true}, nil
}
