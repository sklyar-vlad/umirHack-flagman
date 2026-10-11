package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type serviceStub struct {
	calls int
	err   error
	role  string
}

func (s *serviceStub) Get(_ context.Context, role string) (dto.DashboardResponse, error) {
	s.calls++
	s.role = role
	return dto.DashboardResponse{AttentionBatches: []dto.AttentionBatch{}, HotOffers: []dto.HotOffer{}, ProcurementRecommendations: []dto.ProcurementItem{}, Meta: map[string]any{}}, s.err
}

func serve(role, query string, service *serviceStub) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if role != "" {
			c.Request = c.Request.WithContext(
				requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: role}),
			)
		}
	})
	router.GET("/dashboard", NewHandler(service).Get)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard"+query, http.NoBody))
	return response
}

func TestAuthorizationBeforeParsing(t *testing.T) {
	for _, role := range []string{"", "unknown", "order_operator", "storekeeper", "manager", "admin"} {
		for _, query := range []string{"", "?page=0", "?search=%zz", "?unused=", "?sort=name", "?page=1&page=2"} {
			service := &serviceStub{}
			response := serve(role, query, service)
			want := http.StatusOK
			switch {
			case role == "":
				want = http.StatusUnauthorized
			case role == "unknown":
				want = http.StatusForbidden
			case query != "":
				want = http.StatusUnprocessableEntity
			}
			if response.Code != want || (want != http.StatusOK && service.calls != 0) || (want == http.StatusOK && service.role != role) {
				t.Fatalf("role %q query %q: %d calls %d", role, query, response.Code, service.calls)
			}
		}
	}
}

func TestSafeServiceError(t *testing.T) {
	response := serve("manager", "", &serviceStub{err: errors.New("private database details")})
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("unsafe error: %d %s", response.Code, response.Body.String())
	}
}
