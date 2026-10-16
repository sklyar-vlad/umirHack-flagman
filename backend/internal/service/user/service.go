package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type Repository interface {
	LockAdministration(context.Context, repository.DBTX) error
	CountActiveAdmins(context.Context, repository.DBTX) (int64, error)
	RevokeSessions(context.Context, repository.DBTX, uuid.UUID) error
	List(context.Context, repository.DBTX, model.UserFilter) ([]model.User, int64, error)
	Get(context.Context, repository.DBTX, uuid.UUID, bool) (model.User, error)
	Create(context.Context, repository.DBTX, *model.User, string) (model.User, error)
	Update(context.Context, repository.DBTX, *model.User) (model.User, error)
	SetPassword(context.Context, repository.DBTX, uuid.UUID, string) (model.User, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}

type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	events       EventWriter
}

func NewService(repo Repository, transactions Transactions, events EventWriter) *Service {
	return &Service{repo: repo, transactions: transactions, events: events}
}

func admin(ctx context.Context) (model.Actor, error) {
	actor, ok := requestctx.Actor(ctx)
	if !ok {
		return model.Actor{}, apperrors.ErrUnauthorized
	}
	if actor.Role != roleAdmin {
		return model.Actor{}, apperrors.ErrForbidden
	}
	return actor, nil
}

func validRole(role string) bool {
	return role == roleAdmin || role == "manager" || role == "storekeeper" || role == "order_operator"
}

func userDTO(u *model.User) dto.User {
	if u.LastLoginAt != nil {
		value := u.LastLoginAt.UTC()
		u.LastLoginAt = &value
	}
	return dto.User{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Role:        u.Role,
		IsActive:    u.IsActive,
		Revision:    u.Revision,
		CreatedAt:   u.CreatedAt.UTC(),
		LastLoginAt: u.LastLoginAt,
	}
}

func (s *Service) Roles(ctx context.Context) ([]dto.Role, error) {
	if _, err := admin(ctx); err != nil {
		return nil, err
	}
	return []dto.Role{
		{Code: roleAdmin, Name: "Администратор"},
		{Code: "manager", Name: "Менеджер"},
		{Code: "storekeeper", Name: "Кладовщик"},
		{Code: "order_operator", Name: "Оператор заказов"},
	}, nil
}

func (s *Service) List(ctx context.Context, f model.UserFilter) ([]dto.User, int64, error) {
	if _, err := admin(ctx); err != nil {
		return nil, 0, err
	}
	var users []model.User
	var total int64
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			users, total, err = s.repo.List(ctx, tx, f)
			return err
		},
	)
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.User, 0, len(users))
	for _, u := range users {
		items = append(items, userDTO(&u))
	}
	return items, total, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (dto.User, error) {
	if _, err := admin(ctx); err != nil {
		return dto.User{}, err
	}
	var u model.User
	err := s.transactions.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx repository.DBTX) error {
			var err error
			u, err = s.repo.Get(ctx, tx, id, false)
			return err
		},
	)
	if err != nil {
		return dto.User{}, err
	}
	return userDTO(&u), nil
}
