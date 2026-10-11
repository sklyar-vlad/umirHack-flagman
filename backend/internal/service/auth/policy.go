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
)

// Each implemented business operation is listed explicitly. A future route
// must declare its policy before it can be used, including by an administrator.
var routePermissions = map[string]permission{
	"GET /auth/me":                     warehouse,
	"POST /auth/logout":                warehouse,
	"GET /products":                    warehouse,
	"GET /inventory":                   warehouse,
	"GET /products/:id/batches":        warehouse,
	"GET /dashboard":                   warehouse,
	"GET /supplies":                    supplies,
	"GET /supplies/:id":                supplies,
	"POST /supplies":                   supplies,
	"GET /procurement-recommendations": supplies,
	"GET /dishes":                      menuAndOrders,
	"GET /dishes/:id":                  menuAndOrders,
	"GET /stop-list":                   menuAndOrders,
	"GET /hot-offers":                  menuAndOrders,
	"POST /orders/check":               menuAndOrders,
	"POST /orders":                     menuAndOrders,
	"GET /orders":                      menuAndOrders,
	"GET /orders/:id":                  menuAndOrders,
	"GET /order-attempts":              menuAndOrders,
}

func roleAllowed(role string, permission permission) bool {
	switch role {
	case roleAdmin, roleManager:
		return permission == warehouse || permission == supplies || permission == menuAndOrders
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
	// The basket check is an explicitly read-only POST; it never changes state.
	if method != http.MethodGet && method != http.MethodHead && path != "/orders/check" && !s.ValidMutation(session, origin, csrf) {
		return model.Session{}, apperrors.ErrForbidden
	}
	return session, nil
}
