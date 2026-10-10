package order

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type checkService struct {
	calls    int
	request  dto.OrderRequest
	response dto.OrderCheck
	err      error
	ctx      context.Context
}

func (s *checkService) Check(ctx context.Context, request dto.OrderRequest) (dto.OrderCheck, error) {
	s.calls++
	s.request, s.ctx = request, ctx
	return s.response, s.err
}

type checkAccess struct{ role string }

func (a checkAccess) RequireProductReadAccess(c *gin.Context) {
	if a.role != "" {
		c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: a.role}))
	}
	c.Next()
}

func checkRouter(service *checkService, role string) http.Handler {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(basehandler.RequestID(), basehandler.BodyLimit(1024))
	basehandler.RegisterOrderCheckRoutes(router.Group("/api/v1"), NewHandler(service), checkAccess{role: role})
	return router
}

func TestCheckAuthorizationBeforeBody(t *testing.T) {
	for _, role := range []string{"", "storekeeper", "unknown", "admin", "manager", "order_operator"} {
		t.Run(role, func(t *testing.T) {
			service := &checkService{}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/orders/check", strings.NewReader(`{"items":[]}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			checkRouter(service, role).ServeHTTP(response, request)
			want := http.StatusOK
			if role == "" {
				want = http.StatusUnauthorized
			} else if role == "storekeeper" || role == "unknown" {
				want = http.StatusForbidden
			}
			if response.Code != want || (want != http.StatusOK && service.calls != 0) {
				t.Fatalf("status=%d calls=%d want=%d: %s", response.Code, service.calls, want, response.Body.String())
			}
		})
	}
}

func TestCheckHTTPFailures(t *testing.T) {
	valid := `{"items":[{"dish_id":"` + uuid.NewString() + `","portions":2}]}`
	for _, tc := range []struct {
		name, path, body, contentType, code string
		status                              int
		err                                 error
		calls                               int
	}{
		{"malformed JSON", "", "{", "application/json", "INVALID_JSON", 400, nil, 0},
		{"wrong shape", "", "[]", "application/json", "VALIDATION_ERROR", 422, nil, 0},
		{"fractional portions", "", `{"items":[{"portions":1.5}]}`, "application/json", "VALIDATION_ERROR", 422, nil, 0},
		{"wrong content type", "", valid, "text/plain", "INVALID_JSON", 400, nil, 0},
		{"unknown query", "?unknown=1", valid, "application/json", "VALIDATION_ERROR", 422, nil, 0},
		{"bad query", "?search=%ZZ", valid, "application/json", "VALIDATION_ERROR", 422, nil, 0},
		{"body limit", "", strings.Repeat(" ", 1025), "application/json", "PAYLOAD_TOO_LARGE", 413, nil, 0},
		{"service validation", "", valid, "application/json", "VALIDATION_ERROR", 422, &validations.FieldError{Field: "items[0].dish_id", Message: "unknown"}, 1},
		{"internal", "", valid, "application/json", "INTERNAL_ERROR", 500, errors.New("sensitive SQL error"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &checkService{err: tc.err}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/orders/check"+tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			checkRouter(service, "order_operator").ServeHTTP(response, request)
			var body dto.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Error.Code != tc.code || service.calls != tc.calls || body.RequestID == "" || body.RequestID != response.Header().Get("X-Request-ID") || strings.Contains(response.Body.String(), "sensitive SQL") {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
			}
		})
	}
}

func TestCheckHTTPShortageIs200(t *testing.T) {
	id := uuid.New()
	service := &checkService{response: dto.OrderCheck{
		Items: []dto.OrderItemSnapshot{}, Requirements: []dto.Requirement{}, Shortages: []dto.Shortage{{ProductID: id, ReasonCode: "ingredient_shortage"}},
		Issues: []dto.OrderIssue{}, Meta: map[string]any{},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders/check", strings.NewReader(`{"items":[{"dish_id":"`+id.String()+`","portions":2}]}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response := httptest.NewRecorder()
	checkRouter(service, "order_operator").ServeHTTP(response, request)
	var body dto.OrderCheck
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.CanCreate || len(body.Shortages) != 1 || body.Issues == nil || service.calls != 1 || service.request.Items[0].DishID != id.String() || service.request.Items[0].Portions != 2 || requestctx.ID(service.ctx) != response.Header().Get("X-Request-ID") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
