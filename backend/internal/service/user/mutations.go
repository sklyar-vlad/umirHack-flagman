package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const roleAdmin = "admin"

func field(field string) *validations.FieldError {
	return &validations.FieldError{Field: field, Message: "Некорректное значение"}
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 200 ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return "", field("name")
	}
	return name, nil
}

func validateEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 || !utf8.ValidString(email) ||
		strings.ContainsAny(email, "\x00\r\n") {
		return "", field("email")
	}
	return email, nil
}

func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || n < 12 || n > 128 {
		return field("password")
	}
	return nil
}

func validateRevision(revision int) error {
	if revision < 1 {
		return field("expected_revision")
	}
	return nil
}

// Sanitize deferred PostgreSQL failures too: commit-time constraints may include
// a credential supplied to INSERT/UPDATE in the database error message.
func mutationError(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) {
		return fmt.Errorf("user transaction: database error (SQLSTATE %s)", pgerr.Code)
	}
	return err
}

func (s *Service) Create(ctx context.Context, input dto.CreateUserRequest) (dto.User, error) {
	if _, err := admin(ctx); err != nil {
		return dto.User{}, err
	}
	name, err := validateName(input.Name)
	if err != nil {
		return dto.User{}, err
	}
	email, err := validateEmail(input.Email)
	if err != nil {
		return dto.User{}, err
	}
	if !validRole(input.Role) {
		return dto.User{}, field("role")
	}
	if err := validatePassword(input.Password); err != nil {
		return dto.User{}, err
	}
	hash, err := authservice.Hash(input.Password)
	if err != nil {
		return dto.User{}, err
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	var result model.User
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.lockAdministration(ctx, tx); err != nil {
			return err
		}
		var err error
		result, err = s.repo.Create(
			ctx,
			tx,
			&model.User{Name: name, Email: email, Role: input.Role, IsActive: active},
			hash,
		)
		if err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, nil, &result, model.AuditCreate)
	})
	if err != nil {
		return dto.User{}, mutationError(err)
	}
	return userDTO(&result), nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input dto.UpdateUserRequest) (dto.User, error) {
	if _, err := admin(ctx); err != nil {
		return dto.User{}, err
	}
	if err := validateRevision(input.ExpectedRevision); err != nil {
		return dto.User{}, err
	}
	if input.Name == nil && input.Email == nil && input.Role == nil && input.IsActive == nil {
		return dto.User{}, field("body")
	}
	if input.Name != nil {
		value, err := validateName(*input.Name)
		if err != nil {
			return dto.User{}, err
		}
		input.Name = &value
	}
	if input.Email != nil {
		value, err := validateEmail(*input.Email)
		if err != nil {
			return dto.User{}, err
		}
		input.Email = &value
	}
	if input.Role != nil && !validRole(*input.Role) {
		return dto.User{}, field("role")
	}
	var result model.User
	err := s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.lockAdministration(ctx, tx); err != nil {
			return err
		}
		before, err := s.repo.Get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return &apperrors.RevisionConflict{CurrentRevision: before.Revision}
		}
		next := before
		if input.Name != nil {
			next.Name = *input.Name
		}
		if input.Email != nil {
			next.Email = *input.Email
		}
		if input.Role != nil {
			next.Role = *input.Role
		}
		if input.IsActive != nil {
			next.IsActive = *input.IsActive
		}
		if before.IsActive && before.Role == roleAdmin && (!next.IsActive || next.Role != roleAdmin) {
			count, err := s.repo.CountActiveAdmins(ctx, tx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return apperrors.ErrLastAdmin
			}
		}
		result, err = s.repo.Update(ctx, tx, &next)
		if err != nil {
			return err
		}
		if before.Role != next.Role || (before.IsActive && !next.IsActive) {
			if err := s.repo.RevokeSessions(ctx, tx, id); err != nil {
				return err
			}
		}
		return s.appendAudit(ctx, tx, &before, &result, model.AuditUpdate)
	})
	if err != nil {
		return dto.User{}, mutationError(err)
	}
	return userDTO(&result), nil
}

func (s *Service) Password(ctx context.Context, id uuid.UUID, input dto.UserPasswordRequest) (dto.User, error) {
	if _, err := admin(ctx); err != nil {
		return dto.User{}, err
	}
	if err := validateRevision(input.ExpectedRevision); err != nil {
		return dto.User{}, err
	}
	if err := validatePassword(input.Password); err != nil {
		return dto.User{}, err
	}
	hash, err := authservice.Hash(input.Password)
	if err != nil {
		return dto.User{}, err
	}
	var result model.User
	err = s.transactions.WithinTx(ctx, &pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx repository.DBTX) error {
		if err := s.lockAdministration(ctx, tx); err != nil {
			return err
		}
		before, err := s.repo.Get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if before.Revision != input.ExpectedRevision {
			return &apperrors.RevisionConflict{CurrentRevision: before.Revision}
		}
		result, err = s.repo.SetPassword(ctx, tx, id, hash)
		if err != nil {
			return err
		}
		if err := s.repo.RevokeSessions(ctx, tx, id); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, &before, &result, model.AuditPasswordChange)
	})
	if err != nil {
		return dto.User{}, mutationError(err)
	}
	return userDTO(&result), nil
}

// Read committed gives each statement a fresh snapshot after the advisory wait.
// Recheck the actor as well so a waiting former admin cannot perform a mutation.
func (s *Service) lockAdministration(ctx context.Context, tx repository.DBTX) error {
	if err := s.repo.LockAdministration(ctx, tx); err != nil {
		return err
	}
	actor, err := admin(ctx)
	if err != nil {
		return err
	}
	current, err := s.repo.Get(ctx, tx, actor.ID, false)
	if err != nil {
		return err
	}
	if !current.IsActive || current.Role != roleAdmin {
		return apperrors.ErrForbidden
	}
	return nil
}
