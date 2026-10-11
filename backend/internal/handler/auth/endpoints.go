package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func (h *Handler) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if len(c.Request.Header.Values("Origin")) != 1 || !h.service.AllowedOrigin(c.GetHeader("Origin")) {
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return
	}
	peer, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		peer = c.Request.RemoteAddr
	}
	if !h.limiter.allow("ip", peer, 20) {
		h.loginLimited(c)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		basehandler.WriteError(c, apperrors.ErrInvalidJSON)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if len(body) > 4096 {
		basehandler.WriteError(c, apperrors.ErrBodyTooLarge)
		return
	}
	if !json.Valid(body) || !utf8.Valid(body) {
		basehandler.WriteError(c, apperrors.ErrInvalidJSON)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request dto.LoginRequest
	if err := decoder.Decode(&request); err != nil {
		basehandler.WriteError(c, &validations.FieldError{Field: "body", Message: "Ожидаются email и password"})
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if request.Email == "" || len(request.Email) > 254 || request.Password == "" || utf8.RuneCountInString(request.Password) > 128 {
		basehandler.WriteError(c, &validations.FieldError{Field: "body", Message: "Ожидаются email и пароль до 128 символов"})
		return
	}
	if !h.limiter.allow("email", request.Email, 5) {
		h.loginLimited(c)
		return
	}
	session, token, err := h.service.Authenticate(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	h.setSessionCookie(c, token, session.ExpiresAt)
	c.JSON(http.StatusOK, authResponse(session, false))
}

func (h *Handler) loginLimited(c *gin.Context) {
	c.Header("Retry-After", "900")
	basehandler.WriteError(c, apperrors.ErrTooManyRequests)
}

func (h *Handler) Me(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	session, err := h.currentSession(c)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, authResponse(session, true))
}

func (h *Handler) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	token, ok := sessionToken(c)
	if !ok {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	session, err := h.currentSession(c)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if len(c.Request.Header.Values("Origin")) != 1 || len(c.Request.Header.Values("X-CSRF-Token")) != 1 ||
		!h.service.ValidMutation(session, c.GetHeader("Origin"), c.GetHeader("X-CSRF-Token")) {
		basehandler.WriteError(c, apperrors.ErrForbidden)
		return
	}
	if err := h.service.Revoke(c.Request.Context(), token); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	h.setSessionCookie(c, "", session.ExpiresAt)
	c.Status(http.StatusNoContent)
}

func authResponse(session model.Session, settings bool) dto.AuthResponse {
	user := session.User
	response := dto.AuthResponse{
		User:      dto.User{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, IsActive: user.IsActive, Revision: user.Revision, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt},
		CSRFToken: session.CSRFToken, SessionExpiresAt: session.ExpiresAt,
	}
	if settings {
		response.Settings = &dto.DashboardSettings{RestaurantName: session.Settings.RestaurantName, Timezone: session.Settings.Timezone, ExpiringThresholdHours: session.Settings.ExpiringThresholdHours}
	}
	return response
}
