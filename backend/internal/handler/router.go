package handler

import "github.com/gin-gonic/gin"

type Handler interface {
	Healthcheck(c *gin.Context)
}

func RegisterRoutes(group *gin.RouterGroup, h Handler) {
	group.GET("/healthcheck", h.Healthcheck)
}

type SupplyHandler interface {
	ListSupplies(*gin.Context)
	GetSupply(*gin.Context)
}

func RegisterSupplyRoutes(group *gin.RouterGroup, h SupplyHandler) {
	group.GET("/supplies", h.ListSupplies)
	group.GET("/supplies/:id", h.GetSupply)
}
