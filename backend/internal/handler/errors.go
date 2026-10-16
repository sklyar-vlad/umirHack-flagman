package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

var (
	errBodyTooLarge    = apperrors.ErrBodyTooLarge
	errTooManyRequests = apperrors.ErrTooManyRequests
)

func makeErrorResponse(err error, requestID string) (int, dto.ErrorResponse) {
	return apperrors.MakeResponse(err, requestID)
}

func writeError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	_ = c.Error(err)

	if c.Writer.Written() {
		c.Abort()
		return
	}

	status, response := makeErrorResponse(
		err,
		requestctx.ID(c.Request.Context()),
	)

	if errors.Is(err, apperrors.ErrRequestInProgress) {
		c.Header("Retry-After", "1")
	}
	c.AbortWithStatusJSON(status, response)
}

func NotFound(c *gin.Context) {
	writeError(c, apperrors.ErrNotFound)
}

func WriteError(c *gin.Context, err error) { writeError(c, err) }
