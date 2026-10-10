package order

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestHistoryFilterValidation(t *testing.T) {
	for _, scenario := range []struct{ raw, field string }{
		{"search=%ZZ", "query"}, {"search=a&search=b", "search"},
		{"unknown=a", "unknown"}, {"reason_code=INSUFFICIENT_STOCK", "reason_code"},
		{"page=0", "page"}, {"page=1&page=2", "page"}, {"page_size=101", "page_size"},
		{"page=9223372036854775807&page_size=100", "page"},
		{"from=2026-10-10", "from"}, {"to=", "to"}, {"sort=created_at", "sort"},
		{"from=2026-10-11T00:00:00Z&to=2026-10-10T00:00:00Z", "to"},
	} {
		t.Run(scenario.raw, func(t *testing.T) {
			_, err := parseHistoryFilter(scenario.raw)
			var field *validations.FieldError
			if !errors.As(err, &field) || field.Field != scenario.field {
				t.Fatalf("expected %s validation, got %v", scenario.field, err)
			}
		})
	}
	query := url.Values{"search": {`Pizza%_\`}, "from": {"2026-10-10T12:00:00.000000001+03:00"}, "to": {"2026-10-10T09:00:00.000000001Z"}}
	filter, err := parseHistoryFilter(query.Encode())
	if err != nil || filter.Page != 1 || filter.PageSize != 50 || filter.Search != `Pizza%_\` ||
		filter.DateFrom == nil || filter.DateTo == nil || !filter.DateFrom.Equal(*filter.DateTo) || filter.DateFrom.Location() != time.UTC {
		t.Fatalf("defaults/literal search/UTC interval: %+v %v", filter, err)
	}
}

type historyErrorService struct{ calls int }

func (s *historyErrorService) ListOrders(context.Context, model.OrderHistoryFilter) ([]dto.OrderSummary, int64, error) {
	s.calls++
	return nil, 0, errors.New("private database details")
}

func (s *historyErrorService) GetOrder(context.Context, uuid.UUID) (dto.OrderDetail, error) {
	s.calls++
	return dto.OrderDetail{}, errors.New("private database details")
}

func (s *historyErrorService) ListAttempts(context.Context, model.OrderHistoryFilter) ([]dto.OrderAttempt, int64, error) {
	s.calls++
	return nil, 0, errors.New("private database details")
}

func TestHistoryHandlerAccessPrecedesParsingAndErrorsAreSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"", "storekeeper", "order_operator"} {
		service := &historyErrorService{}
		router := gin.New()
		router.Use(basehandler.RequestID())
		basehandler.RegisterOrderHistoryRoutes(router.Group("/api/v1"), NewHistoryHandler(service), checkAccess{role: role})
		for _, path := range []string{"/orders?unknown=1", "/orders/invalid", "/order-attempts?reason_code=bad"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			want := http.StatusUnprocessableEntity
			if role == "" {
				want = http.StatusUnauthorized
			}
			if role == "storekeeper" {
				want = http.StatusForbidden
			}
			if response.Code != want || service.calls != 0 {
				t.Fatalf("role %s path %s: %d calls %d", role, path, response.Code, service.calls)
			}
		}
		if role != "order_operator" {
			continue
		}
		for _, path := range []string{"/orders", "/orders/" + uuid.NewString(), "/order-attempts"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			requestID := response.Header().Get("X-Request-ID")
			if response.Code != http.StatusInternalServerError || requestID == "" || strings.Contains(response.Body.String(), "private") || !strings.Contains(response.Body.String(), requestID) {
				t.Fatalf("unsafe error: %d %s", response.Code, response.Body.String())
			}
		}
		if service.calls != 3 {
			t.Fatalf("service calls: %d", service.calls)
		}
	}
}
