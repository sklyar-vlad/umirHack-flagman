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

type (
	OrderCreateHandler interface{ Create(*gin.Context) }
	OrderWriteAccess   interface{ RequireOrderAccess(*gin.Context) }
)

func RegisterOrderCreateRoutes(group *gin.RouterGroup, h OrderCreateHandler, access OrderWriteAccess) {
	group.POST("/orders", access.RequireOrderAccess, h.Create)
}

type OrderHistoryHandler interface {
	ListOrders(*gin.Context)
	GetOrder(*gin.Context)
	ListAttempts(*gin.Context)
}

func RegisterOrderHistoryRoutes(group *gin.RouterGroup, h OrderHistoryHandler, access ProductAccess) {
	group.GET("/orders", access.RequireProductReadAccess, h.ListOrders)
	group.GET("/orders/:id", access.RequireProductReadAccess, h.GetOrder)
	group.GET("/order-attempts", access.RequireProductReadAccess, h.ListAttempts)
}

type ProcurementHandler interface{ List(*gin.Context) }

func RegisterProcurementRoutes(group *gin.RouterGroup, h ProcurementHandler, access SupplyAccess) {
	group.GET("/procurement-recommendations", access.RequireSupplyAccess, h.List)
}

type HotOffersHandler interface{ List(*gin.Context) }

func RegisterHotOffersRoutes(group *gin.RouterGroup, h HotOffersHandler, access ProductAccess) {
	group.GET("/hot-offers", access.RequireProductReadAccess, h.List)
}

type DashboardHandler interface{ Get(*gin.Context) }

func RegisterDashboardRoutes(group *gin.RouterGroup, h DashboardHandler, access ProductAccess) {
	group.GET("/dashboard", access.RequireProductReadAccess, h.Get)
}

type AuthHandler interface {
	Login(*gin.Context)
	Me(*gin.Context)
	Logout(*gin.Context)
}

func RegisterAuthRoutes(group *gin.RouterGroup, h AuthHandler) {
	group.POST("/auth/login", h.Login)
	group.GET("/auth/me", h.Me)
	group.POST("/auth/logout", h.Logout)
}

type UserReadHandler interface {
	Roles(*gin.Context)
	List(*gin.Context)
	Get(*gin.Context)
}

func RegisterUserReadRoutes(group *gin.RouterGroup, h UserReadHandler) {
	group.GET("/roles", h.Roles)
	group.GET("/users", h.List)
	group.GET("/users/:id", h.Get)
}

type UserWriteHandler interface {
	Create(*gin.Context)
	Update(*gin.Context)
	Password(*gin.Context)
}

func RegisterUserWriteRoutes(group *gin.RouterGroup, h UserWriteHandler) {
	group.POST("/users", h.Create)
	group.PATCH("/users/:id", h.Update)
	group.POST("/users/:id/password", h.Password)
}

type CatalogHandler interface {
	ListCategories(*gin.Context)
	GetCategory(*gin.Context)
	CreateCategory(*gin.Context)
	UpdateCategory(*gin.Context)
	GetProduct(*gin.Context)
	CreateProduct(*gin.Context)
	UpdateProduct(*gin.Context)
}

func RegisterCatalogRoutes(group *gin.RouterGroup, h CatalogHandler) {
	group.GET("/product-categories", h.ListCategories)
	group.GET("/product-categories/:id", h.GetCategory)
	group.POST("/product-categories", h.CreateCategory)
	group.PATCH("/product-categories/:id", h.UpdateCategory)
	group.GET("/products/:id", h.GetProduct)
	group.POST("/products", h.CreateProduct)
	group.PATCH("/products/:id", h.UpdateProduct)
}
