package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Service will define the business operations used by handlers.
type Service interface{}

type handler struct {
	service Service
	logger  *zap.Logger
}

func NewHandler(service Service, logger *zap.Logger) *handler {
	return &handler{service: service, logger: logger}
}

func (h *handler) Healthcheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "umirHack-flagman",
	})
}
