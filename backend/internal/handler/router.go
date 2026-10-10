package handler

import "github.com/gin-gonic/gin"

type Handler interface {
	Healthcheck(c *gin.Context)
	RequireSupplyAccess(c *gin.Context)
	ListSupplies(c *gin.Context)
	CreateSupply(c *gin.Context)
	GetSupply(c *gin.Context)
}

func RegisterRoutes(group *gin.RouterGroup, h Handler) {
	group.GET("/healthcheck", h.Healthcheck)
	group.GET("/supplies", h.RequireSupplyAccess, h.ListSupplies)
	group.POST("/supplies", h.RequireSupplyAccess, h.CreateSupply)
	group.GET("/supplies/:id", h.RequireSupplyAccess, h.GetSupply)
}

type ProductHandler interface {
	ListProducts(*gin.Context)
}

type ProductAccess interface {
	RequireProductReadAccess(*gin.Context)
}

func RegisterProductRoutes(group *gin.RouterGroup, h ProductHandler, access ProductAccess) {
	group.GET("/products", access.RequireProductReadAccess, h.ListProducts)
}

type InventoryHandler interface {
	ListInventory(*gin.Context)
}

func RegisterInventoryRoutes(group *gin.RouterGroup, h InventoryHandler, access ProductAccess) {
	group.GET("/inventory", access.RequireProductReadAccess, h.ListInventory)
}

type BatchHandler interface {
	ListBatches(*gin.Context)
}

func RegisterBatchRoutes(group *gin.RouterGroup, h BatchHandler, access ProductAccess) {
	group.GET("/products/:id/batches", access.RequireProductReadAccess, h.ListBatches)
}

type MenuHandler interface {
	ListDishes(*gin.Context)
	GetDish(*gin.Context)
	StopList(*gin.Context)
}

func RegisterMenuRoutes(group *gin.RouterGroup, h MenuHandler, access ProductAccess) {
	group.GET("/dishes", access.RequireProductReadAccess, h.ListDishes)
	group.GET("/dishes/:id", access.RequireProductReadAccess, h.GetDish)
	group.GET("/stop-list", access.RequireProductReadAccess, h.StopList)
}

type OrderCheckHandler interface {
	Check(*gin.Context)
}

// Basket checks only read data, so they use session read access. The order
// handler restricts the roles to admin, manager and order_operator.
func RegisterOrderCheckRoutes(group *gin.RouterGroup, h OrderCheckHandler, access ProductAccess) {
	group.POST("/orders/check", access.RequireProductReadAccess, h.Check)
}
