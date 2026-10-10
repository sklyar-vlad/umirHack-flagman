package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
)

type Handler interface {
	Healthcheck(c *gin.Context)
}

func RegisterRoutes(group *gin.RouterGroup, h Handler) {
	group.GET("/healthcheck", h.Healthcheck)
}

func RegisterSupplyRoutes(group *gin.RouterGroup, h *supply.Handler) {
	group.GET("/supplies", h.ListSupplies)
	group.GET("/supplies/:id", h.GetSupply)
}