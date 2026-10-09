package handler

import "github.com/gin-gonic/gin"

type Handler interface {
	Healthcheck(c *gin.Context)
}

func RegisterRoutes(group *gin.RouterGroup, h Handler) {
	group.GET("/healthcheck", h.Healthcheck)
}
