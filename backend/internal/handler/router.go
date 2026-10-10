package handler

import "github.com/gin-gonic/gin"

type Handler interface {
	Healthcheck(c *gin.Context)
	RequireSupplyAccess(c *gin.Context)
	CreateSupply(c *gin.Context)
	GetSupply(c *gin.Context)
}

func RegisterRoutes(group *gin.RouterGroup, h Handler) {
	group.GET("/healthcheck", h.Healthcheck)
	group.POST("/supplies", h.RequireSupplyAccess, h.CreateSupply)
	group.GET("/supplies/:id", h.RequireSupplyAccess, h.GetSupply)
}
