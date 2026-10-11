package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	Get(context.Context, repository.DBTX, bool) (model.RestaurantSettings, error)
	Update(context.Context, repository.DBTX, model.RestaurantSettings) (model.RestaurantSettings, error)
	LockActor(context.Context, repository.DBTX, uuid.UUID) error
}
type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
}
type EventWriter interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}
type Service struct {
	repo         Repository
	transactions Transactions
	events       EventWriter
}

func NewService(repo Repository, transactions Transactions, events EventWriter) *Service {
	return &Service{repo: repo, transactions: transactions, events: events}
}
func actor(ctx context.Context, write bool) (model.Actor, error) {
	a, ok := requestctx.Actor(ctx)
	if !ok || a.ID == uuid.Nil {
		return a, apperrors.ErrUnauthorized
	}
	switch a.Role {
	case "admin":
		return a, nil
	case "manager", "storekeeper", "order_operator":
		if !write {
			return a, nil
		}
	}
	return a, apperrors.ErrForbidden
}
func databaseError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("settings transaction: database error (SQLSTATE %s)", pg.Code)
	}
	return err
}
func (s *Service) Get(ctx context.Context) (dto.Settings, error) {
	if _, err := actor(ctx, false); err != nil {
		return dto.Settings{}, err
	}
	var result model.RestaurantSettings
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx repository.DBTX) error {
		var err error
		result, err = s.repo.Get(ctx, tx, false)
		return err
	})
	if err != nil {
		return dto.Settings{}, databaseError(err)
	}
	return dto.Settings(result), nil
}
func field(name string) error {
	return &validations.FieldError{Field: name, Message: "Некорректное значение"}
}
func validate(input *dto.UpdateSettingsRequest) error {
	if input.ExpectedRevision < 1 || input.ExpectedRevision >= 2147483647 {
		return field("expected_revision")
	}
	if input.RestaurantName == nil && input.Timezone == nil && input.ExpiringThresholdHours == nil && input.AutoWriteOffExpired == nil {
		return field("body")
	}
	if input.RestaurantName != nil {
		name := strings.TrimSpace(*input.RestaurantName)
		if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 200 {
			return field("restaurant_name")
		}
		for _, r := range name {
			if unicode.IsControl(r) {
				return field("restaurant_name")
			}
		}
		input.RestaurantName = &name
	}
	if input.Timezone != nil {
		zone := *input.Timezone
		// Local depends on the host. Absolute paths and path traversal are not IANA names.
		if zone == "" || zone == "Local" || len(zone) > 100 || strings.Contains(zone, "..") || strings.HasPrefix(zone, "/") || strings.ContainsAny(zone, "\\\x00") {
			return field("timezone")
		}
		if _, err := time.LoadLocation(zone); err != nil {
			return field("timezone")
		}
	}
	if input.ExpiringThresholdHours != nil && (*input.ExpiringThresholdHours < 1 || *input.ExpiringThresholdHours > 720) {
		return field("expiring_threshold_hours")
	}
	// The expiry worker is delivered in S21. Never accept a flag with no consumer.
	if input.AutoWriteOffExpired != nil && *input.AutoWriteOffExpired {
		return apperrors.ErrFeatureUnavailable
	}
	return nil
}
func (s *Service) Update(ctx context.Context, input dto.UpdateSettingsRequest) (dto.Settings, error) {
	a, err := actor(ctx, true)
	if err != nil {
		return dto.Settings{}, err
	}
	if err := validate(&input); err != nil {
		return dto.Settings{}, err
	}
	var result model.RestaurantSettings
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.repo.LockActor(ctx, tx, a.ID); err != nil {
			return err
		}
		before, err := s.repo.Get(ctx, tx, true)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return &apperrors.RevisionConflict{CurrentRevision: before.Revision}
		}
		next := before
		if input.RestaurantName != nil {
			next.RestaurantName = *input.RestaurantName
		}
		if input.Timezone != nil {
			next.Timezone = *input.Timezone
		}
		if input.ExpiringThresholdHours != nil {
			next.ExpiringThresholdHours = *input.ExpiringThresholdHours
		}
		if input.AutoWriteOffExpired != nil {
			next.AutoWriteOffExpired = *input.AutoWriteOffExpired
		}
		result, err = s.repo.Update(ctx, tx, next)
		if err != nil {
			return err
		}
		oldSnapshot, newSnapshot := model.SettingsAuditSnapshot(before), model.SettingsAuditSnapshot(result)
		if err := s.events.AppendAudit(ctx, tx, &model.AuditEntry{ActorID: &a.ID, Action: model.AuditUpdate, EntityType: model.AuditSettings, Before: &oldSnapshot, After: &newSnapshot}); err != nil {
			return err
		}
		return s.events.AppendOutbox(ctx, tx, &model.OutboxEvent{ActorID: &a.ID, Type: model.SettingsChanged})
	})
	if err != nil {
		return dto.Settings{}, databaseError(err)
	}
	return dto.Settings(result), nil
}
