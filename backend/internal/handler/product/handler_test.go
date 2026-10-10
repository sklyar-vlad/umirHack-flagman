package product

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
	filter model.ProductFilter
	err    error
}

func (s *serviceStub) ListProducts(_ context.Context, filter model.ProductFilter) ([]dto.Product, int64, error) {
	s.calls++
	s.filter = filter
	return []dto.Product{}, 0, s.err
}

func productRouter(s Service, authenticated bool) *gin.Engine {
	r := gin.New()
	r.Use(handler.RequestID())
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(
				requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: "storekeeper"}),
			)
		})
	}
	r.GET("/api/v1/products", NewHandler(s).ListProducts)
	return r
}

func TestProductListInvalidQueryBeforeService(t *testing.T) {
	for _, query := range []string{
		"unknown=1", "page=0", "page=", "page_size=101", "page_size=x", "sort=-name", "is_active=yes",
		"is_active=", "category_id=bad", "category_id=", "search=x&search=y", "page=1&page=2",
		"category_id=00000000000000000000000000000001", "search=%ZZ", "page=1;page_size=2",
		"page=9223372036854775807&page_size=100",
	} {
		t.Run(query, func(t *testing.T) {
			s := &serviceStub{}
			r := httptest.NewRecorder()
			productRouter(
				s,
				true,
			).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/products?"+query, http.NoBody))
			var response dto.ErrorResponse
			if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if r.Code != http.StatusUnprocessableEntity || s.calls != 0 || response.Error.Code != "VALIDATION_ERROR" ||
				response.RequestID != r.Header().Get("X-Request-ID") {
				t.Fatalf("incorrect validation: %s", r.Body.String())
			}
		})
	}
}

func TestProductListDefaultsAndErrors(t *testing.T) {
	for _, test := range []struct {
		name          string
		authenticated bool
		err           error
		status        int
		code          string
	}{
		{"defaults", true, nil, http.StatusOK, ""},
		{"unauthorized", false, nil, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"database error", true, errors.New("private SQL failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &serviceStub{err: test.err}
			r := httptest.NewRecorder()
			productRouter(
				s,
				test.authenticated,
			).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/products", http.NoBody))
			if r.Code != test.status {
				t.Fatal(r.Body.String())
			}
			if test.status == http.StatusOK {
				var result dto.ProductListResponse
				if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if s.filter.IsActive == nil || !*s.filter.IsActive || s.filter.Page != 1 || s.filter.PageSize != 50 ||
					result.Items == nil || result.Meta == nil {
					t.Fatal("incorrect active/pagination/empty defaults")
				}
				return
			}
			var result dto.ErrorResponse
			if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Error.Code != test.code || strings.Contains(r.Body.String(), "private SQL") ||
				(!test.authenticated && s.calls != 0) {
				t.Fatal("incorrect access or safe error")
			}
		})
	}
}
