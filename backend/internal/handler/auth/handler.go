package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

type Handler struct{ service *authservice.Service }

func NewHandler(service *authservice.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RequireSupplyAccess(c *gin.Context) {
	token, ok := sessionToken(c)
	if !ok || h.service == nil {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	origin, csrf := c.GetHeader("Origin"), c.GetHeader("X-CSRF-Token")
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead &&
		(len(c.Request.Header.Values("Origin")) != 1 || len(c.Request.Header.Values("X-CSRF-Token")) != 1) {
		origin, csrf = "", ""
	}
	actor, err := h.service.AuthorizeSupply(c.Request.Context(), token, c.Request.Method, origin, csrf)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), actor))
	c.Next()
}

func (h *Handler) RequireProductReadAccess(c *gin.Context) {
	token, ok := sessionToken(c)
	if !ok || h.service == nil {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	actor, err := h.service.AuthorizeProductRead(c.Request.Context(), token)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), actor))
	c.Next()
}

func (h *Handler) RequireOrderAccess(c *gin.Context) {
	token, ok := sessionToken(c)
	if !ok || h.service == nil {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	origin, csrf := c.GetHeader("Origin"), c.GetHeader("X-CSRF-Token")
	if len(c.Request.Header.Values("Origin")) != 1 || len(c.Request.Header.Values("X-CSRF-Token")) != 1 {
		origin, csrf = "", ""
	}
	actor, err := h.service.AuthorizeOrder(c.Request.Context(), token, origin, csrf)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), actor))
	c.Next()
}

func sessionToken(c *gin.Context) (string, bool) {
	var token string
	var count int
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name == authservice.SessionCookieName {
			token = cookie.Value
			count++
		}
	}
	return token, count == 1
}
