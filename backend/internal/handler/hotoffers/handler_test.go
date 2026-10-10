package hotoffers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
}

func (s *serviceStub) List(context.Context, model.HotOffersFilter) (dto.HotOffersListResponse, error) {
	s.calls++
	return dto.HotOffersListResponse{Items: []dto.HotOffer{}, Meta: map[string]any{}}, s.err
}

func serve(role, query string, service *serviceStub) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if role != "" {
			c.Request = c.Request.WithContext(requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: role}))
		}
	})
	router.GET("/hot-offers", NewHandler(service).List)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hot-offers"+query, nil))
	return response
}

func TestAuthorizationBeforeParsing(t *testing.T) {
	for _, role := range []string{"", "unknown", "order_operator", "storekeeper", "manager", "admin"} {
		for _, query := range []string{"", "?page=0", "?search=%zz"} {
			service := &serviceStub{}
			response := serve(role, query, service)
			want := http.StatusOK
			if role == "" {
				want = http.StatusUnauthorized
			} else if role == "unknown" || role == "storekeeper" {
				want = http.StatusForbidden
			} else if query != "" {
				want = http.StatusUnprocessableEntity
			}
			if response.Code != want || (want != http.StatusOK && service.calls != 0) {
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

func TestHotOffersFilterValidation(t *testing.T) {
	query, _ := url.ParseQuery("page=2&page_size=3&search=Пицца")
	filter, err := parseFilter(query)
	if err != nil || filter.Page != 2 || filter.PageSize != 3 || filter.Search == nil || *filter.Search != "Пицца" {
		t.Fatalf("parsed filter: %+v %v", filter, err)
	}
	for _, raw := range []string{"sort=name", "sort=", "unknown=1", "page=0", "page_size=101", "page_size=bad", "page=9223372036854775807&page_size=100", "search=a&search=b", "page=1&page=2", "category_id=bad", "is_active=all", "availability=available"} {
		service := &serviceStub{}
		response := serve("manager", "?"+raw, service)
		if response.Code != http.StatusUnprocessableEntity || service.calls != 0 {
			t.Errorf("accepted %q: %d calls %d", raw, response.Code, service.calls)
		}
	}
}
