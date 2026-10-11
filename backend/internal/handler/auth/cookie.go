package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func (h *Handler) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	maxAge := int(authservice.SessionLifetime.Seconds())
	if token == "" {
		maxAge = -1
		expires = time.Unix(1, 0).UTC()
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: authservice.SessionCookieName, Value: token, Path: "/api/v1",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: c.Request.TLS != nil || h.service.SecureCookies(),
		MaxAge: maxAge, Expires: expires,
	})
}
