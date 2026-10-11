package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

const (
	SessionCookieName = "flagman_session"
	SessionLifetime   = 12 * time.Hour
	roleAdmin         = "admin"
	roleManager       = "manager"
	roleStorekeeper   = "storekeeper"
	roleOrderOperator = "order_operator"
)

type Repository interface {
	FindActiveSession(context.Context, string) (model.Session, error)
	FindCredentials(context.Context, string) (model.Credentials, error)
	CreateSession(context.Context, uuid.UUID, string, string, string) (model.Session, error)
	RevokeSession(context.Context, string) error
}

type Service struct {
	repo           Repository
	frontendOrigin string
}

// Authenticate does not return the raw token in an API response. The handler
// places it only in a host-only session cookie.
func (s *Service) Authenticate(ctx context.Context, email, password string) (model.Session, string, error) {
	credentials, err := s.repo.FindCredentials(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil && !errors.Is(err, apperrors.ErrUnauthorized) {
		return model.Session{}, "", err
	}
	encoded := credentials.PasswordHash
	if errors.Is(err, apperrors.ErrUnauthorized) {
		// Same Argon2id work for an unknown email; this is not a login credential.
		encoded = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	valid := Verify(password, encoded)
	if err != nil || !valid || !credentials.User.IsActive {
		return model.Session{}, "", apperrors.ErrUnauthorized
	}
	token, err := randomToken()
	if err != nil {
		return model.Session{}, "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return model.Session{}, "", err
	}
	session, err := s.repo.CreateSession(
		ctx,
		credentials.User.ID,
		credentials.PasswordHash,
		SessionTokenHash(token),
		csrf,
	)
	if err != nil {
		return model.Session{}, "", err
	}
	return session, token, nil
}

func randomToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate authentication token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func (s *Service) CurrentSession(ctx context.Context, token string) (model.Session, error) {
	return s.activeSession(ctx, token)
}

func (s *Service) Revoke(ctx context.Context, token string) error {
	return s.repo.RevokeSession(ctx, SessionTokenHash(token))
}

func NewService(repo Repository, frontendOrigin string) (*Service, error) {
	origin, err := url.Parse(frontendOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" ||
		origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, fmt.Errorf("FRONTEND_ORIGIN must be an HTTP(S) origin")
	}
	return &Service{repo: repo, frontendOrigin: origin.Scheme + "://" + strings.ToLower(origin.Host)}, nil
}

func SessionTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *Service) AuthorizeSupply(ctx context.Context, token, method, origin, csrf string) (model.Actor, error) {
	session, err := s.activeSession(ctx, token)
	if err != nil {
		return model.Actor{}, err
	}
	switch session.Actor.Role {
	case roleAdmin, roleManager, roleStorekeeper:
	default:
		return model.Actor{}, apperrors.ErrForbidden
	}
	if method != http.MethodGet && method != http.MethodHead {
		if origin != s.frontendOrigin || session.CSRFToken == "" ||
			subtle.ConstantTimeCompare([]byte(csrf), []byte(session.CSRFToken)) != 1 {
			return model.Actor{}, apperrors.ErrForbidden
		}
	}
	return session.Actor, nil
}

func (s *Service) AuthorizeProductRead(ctx context.Context, token string) (model.Actor, error) {
	session, err := s.activeSession(ctx, token)
	if err != nil {
		return model.Actor{}, err
	}
	switch session.Actor.Role {
	case roleAdmin, roleManager, roleStorekeeper, roleOrderOperator:
		return session.Actor, nil
	default:
		return model.Actor{}, apperrors.ErrForbidden
	}
}

func (s *Service) AuthorizeOrder(ctx context.Context, token, origin, csrf string) (model.Actor, error) {
	session, err := s.activeSession(ctx, token)
	if err != nil {
		return model.Actor{}, err
	}
	switch session.Actor.Role {
	case roleAdmin, roleManager, roleOrderOperator:
	default:
		return model.Actor{}, apperrors.ErrForbidden
	}
	if origin != s.frontendOrigin || session.CSRFToken == "" ||
		subtle.ConstantTimeCompare([]byte(csrf), []byte(session.CSRFToken)) != 1 {
		return model.Actor{}, apperrors.ErrForbidden
	}
	return session.Actor, nil
}

func (s *Service) activeSession(ctx context.Context, token string) (model.Session, error) {
	if len(token) < 32 || len(token) > 512 {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	return s.repo.FindActiveSession(ctx, SessionTokenHash(token))
}

// The configured public frontend scheme also covers HTTPS terminated by a proxy.
// Arbitrary forwarding headers cannot change the cookie security policy.
func (s *Service) SecureCookies() bool { return strings.HasPrefix(s.frontendOrigin, "https://") }

func (s *Service) AllowedOrigin(origin string) bool { return origin == s.frontendOrigin }
func (s *Service) ValidMutation(session *model.Session, origin, csrf string) bool {
	return s.AllowedOrigin(origin) && session.CSRFToken != "" &&
		subtle.ConstantTimeCompare([]byte(csrf), []byte(session.CSRFToken)) == 1
}
