package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.NewString()
		ctx := requestctx.WithID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()

		requestLogger := logger.With(
			zap.String("request_id", requestctx.ID(c.Request.Context())),
		)

		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		status := c.Writer.Status()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(started)),
		}

		lastError := c.Errors.Last()
		if lastError != nil {
			fields = append(fields, zap.Error(lastError.Err))
		}

		switch {
		case status >= http.StatusInternalServerError || (status < http.StatusBadRequest && lastError != nil):
			requestLogger.Error("http request completed", fields...)
		case status >= http.StatusBadRequest:
			requestLogger.Warn("http request completed", fields...)
		default:
			requestLogger.Info("http request completed", fields...)
		}
	}
}

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error(
					"panic recovered",
					zap.String(
						"request_id",
						requestctx.ID(c.Request.Context()),
					),
					zap.Any("panic", value),
					zap.ByteString("stack", debug.Stack()),
				)
				writeError(c, fmt.Errorf("panic: %v", value))
			}
		}()

		c.Next()
	}
}

func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			writeError(c, errBodyTooLarge)
			return
		}

		if c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}

		reader := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := io.ReadAll(reader)
		_ = reader.Close()

		if err != nil {
			writeError(c, err)
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

func RateLimit(requestsPerSecond int, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(
		rate.Limit(requestsPerSecond),
		burst,
	)

	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/api/v1/healthcheck" {
			c.Next()
			return
		}

		if !limiter.Allow() {
			c.Header("Retry-After", "1")
			writeError(c, errTooManyRequests)
			return
		}

		c.Next()
	}
}
