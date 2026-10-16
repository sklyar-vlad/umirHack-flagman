package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Readiness(check func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := check(ctx); err != nil {
			c.JSON(
				http.StatusServiceUnavailable,
				gin.H{responseStatus: "not_ready", responseService: serviceName},
			)
			return
		}
		c.JSON(http.StatusOK, gin.H{responseStatus: "ready", responseService: serviceName})
	}
}
