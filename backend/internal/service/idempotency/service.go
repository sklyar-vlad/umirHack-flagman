package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
	TryIdempotencyLock(context.Context, repository.DBTX, *model.IdempotencyScope) (bool, error)
	FindIdempotencyResult(context.Context, repository.DBTX, *model.IdempotencyScope) (*model.IdempotencyRecord, error)
	SaveIdempotencyResult(
		context.Context,
		repository.DBTX,
		*model.IdempotencyScope,
		string,
		*model.IdempotencyResponse,
	) (model.IdempotencyResponse, error)
	DeleteExpiredIdempotencyResults(context.Context, repository.DBTX, int) (int64, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

type Outcome struct {
	Response model.IdempotencyResponse
	Rollback bool
}

type Operation func(repository.DBTX) (Outcome, error)

func (s *Service) Execute(
	ctx context.Context,
	scope model.IdempotencyScope,
	body []byte,
	operation Operation,
) (model.IdempotencyResponse, error) {
	if err := validateScope(&scope); err != nil {
		return model.IdempotencyResponse{}, err
	}
	if operation == nil {
		return model.IdempotencyResponse{}, fmt.Errorf("idempotent operation is required")
	}
	hash := sha256.Sum256(body)
	requestHash := hex.EncodeToString(hash[:])
	var response model.IdempotencyResponse
	replay := false
	err := s.repo.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(q repository.DBTX) error {
		acquired, err := s.repo.TryIdempotencyLock(ctx, q, &scope)
		if err != nil {
			return err
		}
		if !acquired {
			return apperrors.ErrRequestInProgress
		}
		record, err := s.repo.FindIdempotencyResult(ctx, q, &scope)
		if err != nil {
			return err
		}
		if record != nil {
			if record.RequestHash != requestHash {
				return apperrors.ErrIdempotencyConflict
			}
			replay = true
			response = record.Response
			return nil
		}
		tx, err := repository.RequireTransaction(q)
		if err != nil {
			return err
		}

		domainTx, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin idempotent operation savepoint: %w", err)
		}
		outcome, err := operation(domainTx)
		if err != nil {
			return err
		}
		if err := validateResponse(&outcome.Response); err != nil {
			return err
		}
		if outcome.Rollback {
			if outcome.Response.Status < http.StatusBadRequest {
				return fmt.Errorf("only a domain refusal may roll back its writes")
			}
			err = domainTx.Rollback(ctx)
		} else {
			err = domainTx.Commit(ctx)
		}
		if err != nil {
			return fmt.Errorf("finish idempotent operation savepoint: %w", err)
		}
		response, err = s.repo.SaveIdempotencyResult(ctx, q, &scope, requestHash, &outcome.Response)
		return err
	})
	if err != nil {
		return model.IdempotencyResponse{}, err
	}
	if !replay && response.Status >= 400 {
		telemetry.Default.Refusal(response.Body)
	}
	return response, nil
}

func validateScope(scope *model.IdempotencyScope) error {
	if scope.UserID == uuid.Nil {
		return apperrors.ErrUnauthorized
	}
	if scope.Key == uuid.Nil {
		return &validations.FieldError{Field: "Idempotency-Key", Message: "Необходим ненулевой UUID ключа"}
	}
	switch scope.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("unsupported idempotency method")
	}
	if !strings.HasPrefix(scope.Path, "/") || strings.HasPrefix(scope.Path, "//") ||
		strings.ContainsAny(scope.Path, "?# \t\r\n\x00") || !utf8.ValidString(scope.Path) ||
		utf8.RuneCountInString(scope.Endpoint()) > 200 {
		return fmt.Errorf("invalid idempotency endpoint")
	}
	return nil
}

func validateResponse(response *model.IdempotencyResponse) error {
	if response.Status < http.StatusOK || response.Status >= http.StatusInternalServerError {
		return fmt.Errorf("only completed 2xx-4xx idempotency responses may be stored")
	}
	if response.Status == http.StatusNoContent || response.Status == http.StatusResetContent ||
		response.Status == http.StatusNotModified {
		if len(response.Body) != 0 {
			return fmt.Errorf("body is forbidden for this response status")
		}
	} else if !json.Valid(
		response.Body,
	) {
		return fmt.Errorf("idempotency response body must be valid JSON")
	}
	if strings.ContainsAny(response.Location, "\r\n\x00") {
		return fmt.Errorf("invalid idempotency Location header")
	}
	return nil
}
