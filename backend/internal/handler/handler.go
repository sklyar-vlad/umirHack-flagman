package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	responseService = "service"
	responseStatus  = "status"
	serviceName     = "umirHack-flagman"
)

type SupplyHandler interface {
	ListSupplies(*gin.Context)
	CreateSupply(*gin.Context)
	GetSupply(*gin.Context)
}

type SupplyAccess interface {
	RequireSupplyAccess(*gin.Context)
}

type handler struct {
	supplies SupplyHandler
	logger   *zap.Logger
	access   SupplyAccess
}

func NewHandler(supplies SupplyHandler, access SupplyAccess, logger *zap.Logger) *handler {
	return &handler{supplies: supplies, access: access, logger: logger}
}

func (h *handler) RequireSupplyAccess(c *gin.Context) { h.access.RequireSupplyAccess(c) }
func (h *handler) ListSupplies(c *gin.Context)        { h.supplies.ListSupplies(c) }
func (h *handler) CreateSupply(c *gin.Context)        { h.supplies.CreateSupply(c) }
func (h *handler) GetSupply(c *gin.Context)           { h.supplies.GetSupply(c) }

func (h *handler) Healthcheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		responseStatus:  "ok",
		responseService: serviceName,
	})
}
