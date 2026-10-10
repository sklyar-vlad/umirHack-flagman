package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
)

type Repository interface {
	WithinTx(ctx context.Context, options *pgx.TxOptions, fn func(tx repository.DBTX) error) error
	AppendAudit(ctx context.Context, tx repository.DBTX, entry *model.AuditEntry) error
	AppendOutbox(ctx context.Context, tx repository.DBTX, event *model.OutboxEvent) error
	ListSupplies(context.Context, model.SupplyFilter) ([]model.Supply, int64, error)
	GetSupplyByID(context.Context, uuid.UUID) (*model.SupplyDetails, error)
}

type service struct {
	repo   Repository
	logger *zap.Logger
}

func NewService(repo Repository, logger *zap.Logger) *service {
	return &service{repo: repo, logger: logger}
}
