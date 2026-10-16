package writeoff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/quantity"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	LockActor(context.Context, repository.DBTX, uuid.UUID) (dto.ActorBrief, error)
	LockBatch(context.Context, repository.DBTX, uuid.UUID) (model.WriteOffBatch, error)
	WriteOff(context.Context, repository.DBTX, *dto.StockMovement, string) error
}
type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}
type Service struct {
	repo        Repository
	events      EventWriter
	idempotency *idempotency.Service
	now         func() time.Time
}

func NewService(repo Repository, events EventWriter, idem *idempotency.Service) *Service {
	return &Service{repo: repo, events: events, idempotency: idem, now: time.Now}
}

func decode(body []byte) (dto.WriteOffRequest, uuid.UUID, int64, error) {
	var input dto.WriteOffRequest
	if !utf8.Valid(body) || !json.Valid(body) {
		return input, uuid.Nil, 0, apperrors.ErrInvalidJSON
	}
	if bytes.TrimSpace(body)[0] != '{' {
		return input, uuid.Nil, 0, field("body", "Ожидается объект списания")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	_, _ = decoder.Token()
	seen := map[string]bool{}
	for decoder.More() {
		token, _ := decoder.Token()
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return input, uuid.Nil, 0, apperrors.ErrInvalidJSON
		}
		if (name != "batch_id" && name != "quantity" && name != "reason") || seen[name] ||
			bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return input, uuid.Nil, 0, field(name, "Неизвестное поле, повтор или null")
		}
		seen[name] = true
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return input, uuid.Nil, 0, field("body", "Ожидаются batch_id, quantity и reason")
	}
	batch, err := validations.ParseUUID("batch_id", input.BatchID)
	if err != nil {
		return input, uuid.Nil, 0, err
	}
	if batch == uuid.Nil {
		return input, batch, 0, field("batch_id", "Необходим ненулевой UUID партии")
	}
	amount, err := validations.ParseQuantity("quantity", input.Quantity)
	if err != nil {
		return input, batch, 0, err
	}
	if amount <= 0 {
		return input, batch, 0, field("quantity", "Количество должно быть положительным")
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if !utf8.ValidString(input.Reason) || utf8.RuneCountInString(input.Reason) < 1 ||
		utf8.RuneCountInString(input.Reason) > 500 ||
		strings.ContainsFunc(input.Reason, unicode.IsControl) {
		return input, batch, 0, field(
			"reason",
			"Необходима причина длиной от 1 до 500 символов без управляющих символов",
		)
	}
	return input, batch, amount, nil
}

func field(name, message string) error { return &validations.FieldError{Field: name, Message: message} }

func (s *Service) CreateIdempotent(
	ctx context.Context,
	actor, key uuid.UUID,
	body []byte,
) (model.IdempotencyResponse, error) {
	if actor == uuid.Nil {
		return model.IdempotencyResponse{}, apperrors.ErrUnauthorized
	}
	input, batch, amount, err := decode(body)
	if err != nil {
		return model.IdempotencyResponse{}, err
	}
	scope := model.IdempotencyScope{UserID: actor, Method: http.MethodPost, Path: "/api/v1/write-offs", Key: key}
	response, err := s.idempotency.Execute(ctx, scope, body, func(tx repository.DBTX) (idempotency.Outcome, error) {
		who, err := s.repo.LockActor(ctx, tx, actor)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		b, err := s.repo.LockBatch(ctx, tx, batch)
		if errors.Is(err, apperrors.ErrNotFound) {
			return refusal(ctx, err)
		}
		if err != nil {
			return idempotency.Outcome{}, err
		}
		if b.Unit == "pcs" && amount%1000 != 0 {
			return refusal(ctx, field("quantity", "Для штук необходимо целое количество"))
		}
		remaining, err := quantity.ParseMilli(b.RemainingQuantity)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		delta := big.NewInt(amount)
		if remaining.Cmp(delta) < 0 {
			return refusal(ctx, apperrors.ErrInsufficientBatchStock)
		}
		m := dto.StockMovement{
			ID:                     uuid.New(),
			BatchID:                batch,
			ProductID:              b.ProductID,
			Type:                   "manual_writeoff",
			DeltaQuantity:          "-" + quantity.FormatMilli(delta),
			RemainingQuantityAfter: quantity.FormatMilli(remaining.Sub(remaining, delta)),
			ProductNameSnapshot:    b.ProductName,
			UnitSnapshot:           b.Unit,
			Actor:                  &who,
			OccurredAt:             s.now().UTC().Truncate(time.Microsecond),
			Note:                   &input.Reason,
		}
		if err := s.repo.WriteOff(ctx, tx, &m, quantity.FormatMilli(delta)); err != nil {
			return idempotency.Outcome{}, err
		}
		snapshot := &model.StockMovementAuditSnapshot{
			BatchID:                m.BatchID,
			Type:                   m.Type,
			DeltaQuantity:          m.DeltaQuantity,
			RemainingQuantityAfter: m.RemainingQuantityAfter,
			ProductNameSnapshot:    m.ProductNameSnapshot,
			UnitSnapshot:           m.UnitSnapshot,
			Note:                   m.Note,
		}
		if err := s.events.AppendAudit(
			ctx,
			tx,
			&model.AuditEntry{
				ActorID:    &actor,
				Action:     model.AuditManualWriteOff,
				EntityType: model.AuditStockMovement,
				EntityID:   &m.ID,
				After:      snapshot,
			},
		); err != nil {
			return idempotency.Outcome{}, err
		}
		if err := s.events.AppendOutbox(
			ctx,
			tx,
			&model.OutboxEvent{
				Type:       model.InventoryChanged,
				ActorID:    &actor,
				ProductIDs: []uuid.UUID{b.ProductID},
				BatchIDs:   []uuid.UUID{batch},
			},
		); err != nil {
			return idempotency.Outcome{}, err
		}
		raw, err := json.Marshal(m)
		if err != nil {
			return idempotency.Outcome{}, err
		}
		return idempotency.Outcome{Response: model.IdempotencyResponse{Status: http.StatusCreated, Body: raw}}, nil
	})
	return response, safeError(err)
}

func refusal(ctx context.Context, err error) (idempotency.Outcome, error) {
	status, response := apperrors.MakeResponse(err, requestctx.ID(ctx))
	body, err := json.Marshal(response)
	return idempotency.Outcome{Response: model.IdempotencyResponse{Status: status, Body: body}, Rollback: true}, err
}

func safeError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("stock movement transaction: database error (SQLSTATE %s)", pg.Code)
	}
	return err
}
