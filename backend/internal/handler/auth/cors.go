package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
)

func (h *Handler) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if path := c.Request.URL.Path; strings.HasPrefix(path, "/api/v1/auth/") || path == "/api/v1/roles" || path == "/api/v1/users" || strings.HasPrefix(path, "/api/v1/users/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Writer.Header().Add("Vary", "Origin")
		origins := c.Request.Header.Values("Origin")
		if len(origins) == 0 {
			c.Next()
			return
		}
		if len(origins) != 1 || !h.service.AllowedOrigin(origins[0]) {
			basehandler.WriteError(c, apperrors.ErrForbidden)
			return
		}
		c.Header("Access-Control-Allow-Origin", origins[0])
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Expose-Headers", "Location, X-Request-ID, Retry-After")
		if c.Request.Method != http.MethodOptions {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
		c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
		method := c.GetHeader("Access-Control-Request-Method")
		if len(c.Request.Header.Values("Access-Control-Request-Method")) != 1 || (method != "GET" && method != "POST" && method != "PATCH" && method != "PUT" && method != "DELETE") {
			basehandler.WriteError(c, apperrors.ErrForbidden)
			return
		}
		for _, line := range c.Request.Header.Values("Access-Control-Request-Headers") {
			for _, name := range strings.Split(line, ",") {
				switch strings.ToLower(strings.TrimSpace(name)) {
				case "content-type", "x-csrf-token", "x-request-id", "idempotency-key":
				default:
					basehandler.WriteError(c, apperrors.ErrForbidden)
					return
				}
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Request-ID, Idempotency-Key")
		c.AbortWithStatus(http.StatusNoContent)
	}
}
