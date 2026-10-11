package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

const verifiedSessionKey = "flagman.verified_session"

func (h *Handler) RequireRouteAccess(c *gin.Context) {
	path := strings.TrimPrefix(c.FullPath(), "/api/v1")
	if strings.HasPrefix(path, "/auth/") || path == "/settings" || path == "/audit-log" || path == "/roles" || path == "/users" || strings.HasPrefix(path, "/users/") {
		c.Header("Cache-Control", "no-store")
	}
	if (c.Request.Method == http.MethodGet && (path == "/healthcheck" || path == "/ready")) ||
		(c.Request.Method == http.MethodPost && path == "/auth/login") {
		c.Next()
		return
	}
	token, ok := sessionToken(c)
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	origin, csrf := c.GetHeader("Origin"), c.GetHeader("X-CSRF-Token")
	if len(c.Request.Header.Values("Origin")) != 1 || len(c.Request.Header.Values("X-CSRF-Token")) != 1 {
		origin, csrf = "", ""
	}
	session, err := h.service.AuthorizeRoute(c.Request.Context(), token, c.Request.Method, path, origin, csrf)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Set(verifiedSessionKey, session)
	c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), session.Actor))
	c.Next()
}

func (h *Handler) currentSession(c *gin.Context) (model.Session, error) {
	if verified, ok := c.Get(verifiedSessionKey); ok {
		return verified.(model.Session), nil
	}
	token, ok := sessionToken(c)
	if !ok {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	return h.service.CurrentSession(c.Request.Context(), token)
}
