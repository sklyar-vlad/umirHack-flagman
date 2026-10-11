package auditlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type Repository interface {
	List(context.Context, repository.DBTX, *model.AuditLogFilter) ([]model.AuditLogRecord, int64, error)
}
type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type Service struct {
	repo         Repository
	transactions Transactions
}

func NewService(repo Repository, transactions Transactions) *Service {
	return &Service{repo: repo, transactions: transactions}
}

func (s *Service) List(ctx context.Context, f *model.AuditLogFilter) ([]dto.AuditLogEntry, int64, error) {
	actor, ok := requestctx.Actor(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, 0, apperrors.ErrUnauthorized
	}
	if actor.Role != "admin" {
		return nil, 0, apperrors.ErrForbidden
	}
	result := make([]dto.AuditLogEntry, 0)
	var total int64
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			records, count, err := s.repo.List(ctx, tx, f)
			if err != nil {
				return err
			}
			total = count
			for i := range records {
				r := &records[i]
				before, err := safeSnapshot(r.EntityType, r.Before)
				if err != nil {
					return err
				}
				after, err := safeSnapshot(r.EntityType, r.After)
				if err != nil {
					return err
				}
				v := dto.AuditLogEntry{
					ID:         r.ID,
					CreatedAt:  r.CreatedAt.UTC().Format(time.RFC3339Nano),
					Action:     string(r.Action),
					EntityType: string(r.EntityType),
					EntityID:   r.EntityID,
					RequestID:  r.RequestID,
					Before:     before,
					After:      after,
				}
				if r.ActorID != nil && r.ActorName != nil {
					v.Actor = &dto.AuditActor{ID: *r.ActorID, Name: *r.ActorName}
				}
				result = append(result, v)
			}
			return nil
		},
	)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			err = fmt.Errorf("audit log transaction: database error (SQLSTATE %s)", pg.Code)
		}
		return nil, 0, err
	}
	return result, total, nil
}

// Decode through the same typed allowlist as the writer, including nested
// recipes and batches. Extra legacy fields can never expose credentials.
func safeSnapshot(entity model.AuditEntity, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var snapshot model.AuditSnapshot
	switch entity {
	case model.AuditProductCategory:
		snapshot = &model.CategoryAuditSnapshot{}
	case model.AuditProduct:
		snapshot = &model.ProductAuditSnapshot{}
	case model.AuditDish:
		snapshot = &model.DishAuditSnapshot{}
	case model.AuditSupply:
		snapshot = &model.SupplyAuditSnapshot{}
	case model.AuditOrder:
		snapshot = &model.OrderAuditSnapshot{}
	case model.AuditStockMovement:
		snapshot = &model.StockMovementAuditSnapshot{}
	case model.AuditUser:
		snapshot = &model.UserAuditSnapshot{}
	case model.AuditSettings:
		snapshot = &model.SettingsAuditSnapshot{}
	default:
		return nil, nil
	}
	if err := json.Unmarshal(raw, snapshot); err != nil {
		return nil, errors.New("invalid stored audit snapshot")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, errors.New("encode safe audit snapshot")
	}
	return data, nil
}
