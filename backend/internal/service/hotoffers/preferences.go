package hotoffers

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	offersrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type PreferenceTransactions interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}
type PreferenceService struct{ tx PreferenceTransactions }

func NewPreferenceService(tx PreferenceTransactions) *PreferenceService {
	return &PreferenceService{tx: tx}
}

func (s *PreferenceService) Save(
	ctx context.Context,
	id uuid.UUID,
	input dto.UpdateHotOffer,
) (model.HotOfferPreference, error) {
	a, ok := requestctx.Actor(ctx)
	if !ok {
		return model.HotOfferPreference{}, apperrors.ErrUnauthorized
	}
	if a.Role != roleAdmin && a.Role != roleManager {
		return model.HotOfferPreference{}, apperrors.ErrForbidden
	}
	if input.Published == nil || input.Manual == nil || input.ExpectedRevision < 0 ||
		!utf8.ValidString(input.Message) ||
		utf8.RuneCountInString(input.Message) > 500 ||
		strings.ContainsRune(input.Message, 0) {
		return model.HotOfferPreference{}, &validations.FieldError{
			Field:   "body",
			Message: "Укажите публикацию, ручной выбор, ревизию и сообщение до 500 символов",
		}
	}
	p := model.HotOfferPreference{
		Published: *input.Published,
		Manual:    *input.Manual,
		Message:   strings.TrimSpace(input.Message),
	}
	err := s.tx.WithinTx(ctx, nil, func(q repository.DBTX) error {
		before, err := offersrepo.NewRepository().Preferences(ctx, q)
		if err != nil {
			return err
		}
		p, err = offersrepo.NewRepository().SavePreference(ctx, q, id, a.ID, p, input.ExpectedRevision)
		if err != nil {
			return err
		}
		entry := &model.AuditEntry{
			ActorID:    &a.ID,
			Action:     model.AuditCreate,
			EntityType: model.AuditHotOffer,
			EntityID:   &id,
			After:      preferenceSnapshot(p),
		}
		if previous, ok := before[id]; ok {
			entry.Action = model.AuditUpdate
			entry.Before = preferenceSnapshot(previous)
		}
		if err := s.tx.AppendAudit(ctx, q, entry); err != nil {
			return err
		}
		return s.tx.AppendOutbox(
			ctx,
			q,
			&model.OutboxEvent{Type: model.DishesChanged, ActorID: &a.ID, DishIDs: []uuid.UUID{id}},
		)
	})
	return p, err
}

func preferenceSnapshot(p model.HotOfferPreference) *model.WorkflowAuditSnapshot {
	return &model.WorkflowAuditSnapshot{
		Published: &p.Published,
		Manual:    &p.Manual,
		Message:   p.Message,
		Revision:  p.Revision,
	}
}

func (s *PreferenceService) Get(ctx context.Context, id uuid.UUID) (model.HotOfferPreference, error) {
	a, ok := requestctx.Actor(ctx)
	if !ok {
		return model.HotOfferPreference{}, apperrors.ErrUnauthorized
	}
	if a.Role != roleAdmin && a.Role != roleManager {
		return model.HotOfferPreference{}, apperrors.ErrForbidden
	}
	var result model.HotOfferPreference
	err := s.tx.WithinTx(
		ctx,
		&pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(q repository.DBTX) error {
			var err error
			result, err = offersrepo.NewRepository().Preference(ctx, q, id)
			return err
		},
	)
	return result, err
}
