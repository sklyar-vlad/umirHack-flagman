package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

type Repository interface {
	WithinTx(ctx context.Context, options *pgx.TxOptions, fn func(tx repository.DBTX) error) error
	AppendAudit(ctx context.Context, tx repository.DBTX, entry *model.AuditEntry) error
	AppendOutbox(ctx context.Context, tx repository.DBTX, event *model.OutboxEvent) error
}

type service struct {
	repo     Repository
	logger   *zap.Logger
	supplies *supplyservice.Service
}

func NewService(repo Repository, logger *zap.Logger, supplies *supplyservice.Service) *service {
	return &service{repo: repo, logger: logger, supplies: supplies}
}

func (s *service) CreateSupply(
	ctx context.Context,
	actorID uuid.UUID,
	input dto.CreateSupplyRequest,
) (model.CreatedSupply, error) {
	return s.supplies.Create(ctx, actorID, input)
}

func (s *service) CreateSupplyIdempotent(
	ctx context.Context,
	actorID uuid.UUID,
	key uuid.UUID,
	body []byte,
) (model.IdempotencyResponse, error) {
	return s.supplies.Receive(ctx, actorID, key, body)
}

func (s *service) GetSupply(ctx context.Context, id uuid.UUID) (dto.SupplyDetail, error) {
	return s.supplies.Get(ctx, id)
}

func (s *service) ListSupplies(
	ctx context.Context,
	filter model.SupplyFilter,
) ([]supplydto.SupplySummaryDTO, int64, error) {
	return s.supplies.List(ctx, filter)
}
