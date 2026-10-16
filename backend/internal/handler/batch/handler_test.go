package batch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	filter model.BatchFilter
}

func (s *serviceStub) ListBatches(_ context.Context, filter model.BatchFilter) (*dto.BatchListResponse, error) {
	s.calls++
	s.filter = filter
	return &dto.BatchListResponse{}, nil
}

func batchRouter(s Service, authenticated bool) *gin.Engine {
	r := gin.New()
	r.Use(handler.RequestID())
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(
				requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: "order_operator"}),
			)
		})
	}
	r.GET("/api/v1/products/:id/batches", NewHandler(s).ListBatches)
	return r
}

func TestBatchListRejectsInvalidInputBeforeService(t *testing.T) {
	id := uuid.NewString()
	for _, input := range []struct {
		id, query string
	}{
		{"bad", ""},
		{"00000000000000000000000000000001", ""},
		{id, "search=x"},
		{id, "status="},
		{id, "status=nonzero"},
		{id, "status=normal"},
		{id, "status=valid&status=expired"},
		{id, "supply_id=bad"},
		{id, "supply_id="},
		{id, "sort=-expires_at"},
		{id, "sort=created_at"},
		{id, "page=0"},
		{id, "page_size=101"},
		{id, "page=1&page=2"},
		{id, "page=9223372036854775807&page_size=100"},
		{id, "status=%ZZ"},
		{id, "page=1;page_size=2"},
	} {
		t.Run(input.id+"?"+input.query, func(t *testing.T) {
			s := &serviceStub{}
			response := httptest.NewRecorder()
			batchRouter(s, true).ServeHTTP(response, httptest.NewRequest(
				http.MethodGet, "/api/v1/products/"+input.id+"/batches?"+input.query, http.NoBody,
			))
			var result dto.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusUnprocessableEntity || s.calls != 0 ||
				result.Error.Code != "VALIDATION_ERROR" ||
				result.RequestID != response.Header().Get("X-Request-ID") {
				t.Fatalf("invalid input reached service: %s", response.Body.String())
			}
		})
	}
}

func TestBatchListDefaultsFiltersAndMissingActor(t *testing.T) {
	id, supply := uuid.New(), uuid.New()
	for _, test := range []struct {
		name, query   string
		authenticated bool
		status        int
	}{
		{"default nonzero", "", true, http.StatusOK},
		{"all includes depleted", "status=all&sort=-created_at&supply_id=" + supply.String(), true, http.StatusOK},
		{"missing actor", "", false, http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &serviceStub{}
			response := httptest.NewRecorder()
			batchRouter(s, test.authenticated).ServeHTTP(response, httptest.NewRequest(
				http.MethodGet, "/api/v1/products/"+id.String()+"/batches?"+test.query, http.NoBody,
			))
			if response.Code != test.status {
				t.Fatal(response.Body.String())
			}
			if !test.authenticated {
				if s.calls != 0 {
					t.Fatal("unauthenticated request read batches")
				}
				return
			}
			if s.filter.ProductID != id || s.filter.Page != 1 || s.filter.PageSize != 50 {
				t.Fatalf("incorrect path/pagination defaults: %+v", s.filter)
			}
			if test.query == "" {
				if s.filter.Status != "" || s.filter.Sort != "expires_at" || s.filter.SupplyID != nil {
					t.Fatal("default must exclude depleted and sort by expiry")
				}
			} else if s.filter.Status != "all" || s.filter.Sort != "-created_at" || s.filter.SupplyID == nil || *s.filter.SupplyID != supply {
				t.Fatalf("explicit filters lost: %+v", s.filter)
			}
		})
	}
}
