package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

const SessionCookieName = "flagman_session"

type Repository interface {
	FindActiveSession(context.Context, string) (model.Session, error)
}

type Service struct {
	repo           Repository
	frontendOrigin string
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
	case "admin", "manager", "storekeeper":
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
	case "admin", "manager", "storekeeper", "order_operator":
		return session.Actor, nil
	default:
		return model.Actor{}, apperrors.ErrForbidden
	}
}

func (s *Service) activeSession(ctx context.Context, token string) (model.Session, error) {
	if len(token) < 32 || len(token) > 512 {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	return s.repo.FindActiveSession(ctx, SessionTokenHash(token))
}
