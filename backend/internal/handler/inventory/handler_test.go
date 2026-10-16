package inventory

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
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type serviceStub struct {
	calls  int
	filter model.InventoryFilter
	err    error
}

func (s *serviceStub) ListInventory(
	_ context.Context,
	filter model.InventoryFilter,
) (*dto.InventoryListResponse, error) {
	s.calls++
	s.filter = filter
	return &dto.InventoryListResponse{}, s.err
}

func inventoryRouter(s Service, authenticated bool) *gin.Engine {
	r := gin.New()
	r.Use(handler.RequestID())
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(
				requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: "storekeeper"}),
			)
		})
	}
	r.GET("/api/v1/inventory", NewHandler(s).ListInventory)
	return r
}

func TestInventoryInvalidQueryDoesNotReadStock(t *testing.T) {
	for _, query := range []string{
		"unknown=1", "is_active=true", "page=0", "page=", "page_size=101", "page_size=x", "sort=-name",
		"filter=bad", "filter=", "category_id=bad", "category_id=", "search=x&search=y", "page=1&page=2",
		"category_id=00000000000000000000000000000001", "search=%ZZ", "page=1;page_size=2",
		"page=9223372036854775807&page_size=100",
	} {
		t.Run(query, func(t *testing.T) {
			s := &serviceStub{}
			response := httptest.NewRecorder()
			inventoryRouter(s, true).ServeHTTP(response, httptest.NewRequest(
				http.MethodGet, "/api/v1/inventory?"+query, http.NoBody,
			))
			var result dto.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusUnprocessableEntity || s.calls != 0 ||
				result.Error.Code != "VALIDATION_ERROR" ||
				result.RequestID != response.Header().Get("X-Request-ID") {
				t.Fatalf("invalid query reached service: %s", response.Body.String())
			}
		})
	}
}

func TestInventoryDefaultsAndSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name          string
		authenticated bool
		err           error
		status        int
		code          string
	}{
		{"defaults", true, nil, http.StatusOK, ""},
		{"unauthorized", false, nil, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"read failure", true, errors.New("private SQL failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &serviceStub{err: test.err}
			response := httptest.NewRecorder()
			inventoryRouter(s, test.authenticated).ServeHTTP(response, httptest.NewRequest(
				http.MethodGet, "/api/v1/inventory", http.NoBody,
			))
			if response.Code != test.status {
				t.Fatal(response.Body.String())
			}
			if test.status == http.StatusOK {
				if s.filter.Page != 1 || s.filter.PageSize != 50 || s.filter.Sort != "name" ||
					s.filter.Filter != "all" {
					t.Fatalf("incorrect defaults: %+v", s.filter)
				}
				return
			}
			var result dto.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Error.Code != test.code || strings.Contains(response.Body.String(), "private SQL") ||
				(!test.authenticated && s.calls != 0) {
				t.Fatal("incorrect access or unsafe error")
			}
		})
	}
}
