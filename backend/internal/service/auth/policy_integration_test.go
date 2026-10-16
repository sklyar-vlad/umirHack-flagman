package auth_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type routeProbe struct{ access *authhandler.Handler }

func (p routeProbe) ok(c *gin.Context) {
	if _, ok := requestctx.Actor(c.Request.Context()); !ok {
		c.Status(500)
		return
	}
	c.Status(204)
}
func (p routeProbe) Healthcheck(c *gin.Context)         { c.Status(200) }
func (p routeProbe) RequireSupplyAccess(c *gin.Context) { p.access.RequireSupplyAccess(c) }
func (p routeProbe) ListSupplies(c *gin.Context)        { p.ok(c) }
func (p routeProbe) CreateSupply(c *gin.Context)        { p.ok(c) }
func (p routeProbe) GetSupply(c *gin.Context)           { p.ok(c) }
func (p routeProbe) ListProducts(c *gin.Context)        { p.ok(c) }
func (p routeProbe) ListInventory(c *gin.Context)       { p.ok(c) }
func (p routeProbe) ListBatches(c *gin.Context)         { p.ok(c) }
func (p routeProbe) ListDishes(c *gin.Context)          { p.ok(c) }
func (p routeProbe) GetDish(c *gin.Context)             { p.ok(c) }
func (p routeProbe) StopList(c *gin.Context)            { p.ok(c) }
func (p routeProbe) Check(c *gin.Context)               { p.ok(c) }
func (p routeProbe) Create(c *gin.Context)              { p.ok(c) }
func (p routeProbe) ListOrders(c *gin.Context)          { p.ok(c) }
func (p routeProbe) GetOrder(c *gin.Context)            { p.ok(c) }
func (p routeProbe) ListAttempts(c *gin.Context)        { p.ok(c) }
func (p routeProbe) List(c *gin.Context)                { p.ok(c) }
func (p routeProbe) Get(c *gin.Context)                 { p.ok(c) }

func protectedRouter(f *fixture) http.Handler {
	router := gin.New()
	router.Use(basehandler.RequestID())
	access := authhandler.NewHandler(f.service)
	router.Use(access.CORS())
	api := router.Group("/api/v1", access.RequireRouteAccess)
	probe := routeProbe{access: access}
	basehandler.RegisterAuthRoutes(api, access)
	basehandler.RegisterRoutes(api, probe)
	basehandler.RegisterProductRoutes(api, probe, access)
	basehandler.RegisterInventoryRoutes(api, probe, access)
	basehandler.RegisterBatchRoutes(api, probe, access)
	basehandler.RegisterMenuRoutes(api, probe, access)
	basehandler.RegisterOrderCheckRoutes(api, probe, access)
	basehandler.RegisterOrderCreateRoutes(api, probe, access)
	basehandler.RegisterOrderHistoryRoutes(api, probe, access)
	basehandler.RegisterProcurementRoutes(api, probe, access)
	basehandler.RegisterHotOffersRoutes(api, probe, access)
	basehandler.RegisterDashboardRoutes(api, probe, access)

	api.GET("/new-business-route", probe.ok)
	return router
}

func TestFixedRoleMatrixForAllRegisteredBusinessRoutes(t *testing.T) {
	f := newFixture(t)
	router := protectedRouter(f)
	routes := []struct{ method, path, denied string }{
		{"GET", "/products", ""},
		{"GET", "/inventory", ""},
		{"GET", "/products/00000000-0000-0000-0000-000000000001/batches", ""},
		{"GET", "/dashboard", ""},
		{"GET", "/supplies", "order_operator"},
		{"GET", "/supplies/00000000-0000-0000-0000-000000000001", "order_operator"},
		{"POST", "/supplies", "order_operator"},
		{"GET", "/procurement-recommendations", "order_operator"},
		{"GET", "/dishes", "storekeeper"},
		{"GET", "/dishes/00000000-0000-0000-0000-000000000001", "storekeeper"},
		{"GET", "/stop-list", "storekeeper"},
		{"GET", "/hot-offers", "storekeeper"},
		{"POST", "/orders/check", "storekeeper"},
		{"POST", "/orders", "storekeeper"},
		{"GET", "/orders", "storekeeper"},
		{"GET", "/orders/00000000-0000-0000-0000-000000000001", "storekeeper"},
		{"GET", "/order-attempts", "storekeeper"},
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		cookie := &http.Cookie{Name: "flagman_session", Value: token}
		for _, route := range routes {
			expected := 204
			if route.denied == role {
				expected = 403
			}
			w := authRequest(router, route.method, route.path, "", "https://flagman.example", cookie, session.CSRFToken)
			if w.Code != expected {
				t.Fatalf("%s %s %s: status %d", role, route.method, route.path, w.Code)
			}
			w = authRequest(router, route.method, route.path, "", "", nil, "")
			if w.Code != 401 {
				t.Fatalf("public business route %s %s", route.method, route.path)
			}
		}
		if w := authRequest(router, "GET", "/new-business-route", "", "", cookie, ""); w.Code != 403 {
			t.Fatal("undeclared route allowed")
		}
		me := decodeAuth(t, authRequest(router, "GET", "/auth/me", "", "", cookie, ""))
		if me.User.Role != role {
			t.Fatal("role mismatch")
		}
	}
	if w := authRequest(router, "GET", "/healthcheck", "", "", nil, ""); w.Code != 200 {
		t.Fatal("health requires authentication")
	}
}

func TestRouteMiddlewareProtectsMutationsAndPreservesReadOnlyChecks(t *testing.T) {
	f := newFixture(t)
	router := protectedRouter(f)
	_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "flagman_session", Value: token}
	for _, path := range []string{"/supplies", "/orders", "/auth/logout"} {
		for _, input := range [][2]string{{"", ""}, {"https://flagman.example", ""}, {"https://flagman.example", "wrong"}} {
			if w := authRequest(router, "POST", path, "", input[0], cookie, input[1]); w.Code != 403 {
				t.Fatalf("mutation %s bypassed CSRF", path)
			}
		}
	}
	if w := authRequest(router, "POST", "/orders/check", "", "", cookie, ""); w.Code != 204 {
		t.Fatal("read-only basket check requires mutation credentials")
	}
	if w := authRequest(router, "GET", "/dashboard", "", "", cookie, ""); w.Code != 204 {
		t.Fatal("read requires CSRF")
	}
}
