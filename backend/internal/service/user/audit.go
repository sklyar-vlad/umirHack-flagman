package user

import (
	"context"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

// The shared helper accepts only this typed allowlist; neither credentials nor
// session secrets can be included in an administration audit snapshot.
func userSnapshot(u *model.User) *model.UserAuditSnapshot {
	return &model.UserAuditSnapshot{
		Name:     u.Name,
		Email:    u.Email,
		Role:     u.Role,
		IsActive: u.IsActive,
		Revision: u.Revision,
	}
}

func (s *Service) appendAudit(
	ctx context.Context,
	tx repository.DBTX,
	before *model.User,
	after *model.User,
	action model.AuditAction,
) error {
	actor, err := admin(ctx)
	if err != nil {
		return err
	}
	entry := &model.AuditEntry{
		ActorID:    &actor.ID,
		Action:     action,
		EntityType: model.AuditUser,
		EntityID:   &after.ID,
		After:      userSnapshot(after),
	}
	if before != nil {
		entry.Before = userSnapshot(before)
	}
	return s.events.AppendAudit(ctx, tx, entry)
}
