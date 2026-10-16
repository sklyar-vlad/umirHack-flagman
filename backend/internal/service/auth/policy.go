package auth

import (
	"context"
	"net/http"

	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

type permission uint8

const (
	warehouse permission = iota + 1
	supplies
	menuAndOrders
	administration
	analytics
	management
)

var routePermissions = map[string]permission{
	"GET /integration-tokens":             administration,
	"POST /integration-tokens":            administration,
	"DELETE /integration-tokens/:id":      administration,
	"GET /incoming-supplies":              supplies,
	"POST /incoming-supplies/:id/accept":  supplies,
	"GET /hot-offers/:id":                 management,
	"PATCH /hot-offers/:id":               management,
	"GET /analytics/product-flow":         analytics,
	"GET /events":                         warehouse,
	"GET /analytics/stop-list-forecast":   analytics,
	"GET /analytics/ingredient-shortages": analytics,
	"GET /analytics/write-offs":           analytics,
	"GET /stock-movements":                supplies,
	"POST /write-offs":                    supplies,
	"GET /audit-log":                      administration,
	"GET /settings":                       warehouse,
	"PATCH /settings":                     administration,
	"POST /dishes":                        administration,
	"PATCH /dishes/:id":                   administration,
	"POST /dishes/preview":                administration,
	"GET /product-categories":             warehouse,
	"GET /product-categories/:id":         warehouse,
	"GET /products/:id":                   warehouse,
	"POST /product-categories":            administration,
	"PATCH /product-categories/:id":       administration,
	"POST /products":                      administration,
	"PATCH /products/:id":                 administration,
	"POST /users":                         administration,
	"PATCH /users/:id":                    administration,
	"POST /users/:id/password":            administration,
	"GET /roles":                          administration,
	"GET /users":                          administration,
	"GET /users/:id":                      administration,
	"GET /auth/me":                        warehouse,
	"POST /auth/logout":                   warehouse,
	"GET /products":                       warehouse,
	"GET /inventory":                      warehouse,
	"GET /products/:id/batches":           warehouse,
	"GET /dashboard":                      warehouse,
	"GET /supplies":                       supplies,
	"GET /supplies/:id":                   supplies,
	"POST /supplies":                      supplies,
	"GET /procurement-recommendations":    supplies,
	"GET /dishes":                         menuAndOrders,
	"GET /dishes/:id":                     menuAndOrders,
	"GET /stop-list":                      menuAndOrders,
	"GET /hot-offers":                     menuAndOrders,
	"POST /orders/check":                  menuAndOrders,
	"POST /orders":                        menuAndOrders,
	"GET /orders":                         menuAndOrders,
	"GET /orders/:id":                     menuAndOrders,
	"GET /order-attempts":                 menuAndOrders,
}

func roleAllowed(role string, permission permission) bool {
	switch role {
	case roleAdmin:
		return permission == warehouse || permission == supplies || permission == menuAndOrders ||
			permission == administration ||
			permission == analytics || permission == management
	case roleManager:
		return permission == warehouse || permission == supplies || permission == menuAndOrders ||
			permission == analytics || permission == management
	case roleStorekeeper:
		return permission == warehouse || permission == supplies
	case roleOrderOperator:
		return permission == warehouse || permission == menuAndOrders
	default:
		return false
	}
}

func (s *Service) AuthorizeRoute(ctx context.Context, token, method, path, origin, csrf string) (model.Session, error) {
	session, err := s.activeSession(ctx, token)
	if err != nil {
		return model.Session{}, err
	}
	permission, exists := routePermissions[method+" "+path]
	if !exists || !roleAllowed(session.Actor.Role, permission) {
		return model.Session{}, apperrors.ErrForbidden
	}

	if method != http.MethodGet && method != http.MethodHead && path != "/orders/check" &&
		!s.ValidMutation(&session, origin, csrf) {
		return model.Session{}, apperrors.ErrForbidden
	}
	return session, nil
}
