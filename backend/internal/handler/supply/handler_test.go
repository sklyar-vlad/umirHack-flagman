package supply

import (
	"context"
	"encoding/json"
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
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

type listServiceStub struct {
	Service
	filter model.SupplyFilter
	calls  int
	err    error
}

func (s *listServiceStub) ListSupplies(
	_ context.Context,
	filter model.SupplyFilter,
) ([]supplydto.SupplySummaryDTO, int64, error) {
	s.calls++
	s.filter = filter
	return []supplydto.SupplySummaryDTO{}, 0, s.err
}

func listRouter(s Service) *gin.Engine {
	router := gin.New()
	router.Use(handler.RequestID())
	handler.RegisterSupplyRoutes(router.Group("/api/v1"), NewHandler(s))
	return router
}

func TestSupplyListRejectsInvalidQueryBeforeService(t *testing.T) {
	for _, query := range []string{
		"unknown=1", "page=1&page=2", "search=a&search=b", "from=a&from=b", "sort=-created_at&sort=-created_at",
		"page=", "page=0", "page=-1", "page=abc", "page_size=0", "page_size=101",
		"page=9223372036854775807&page_size=100", "sort=received_at", "sort=created_at",
		"from=", "to=invalid", "from=2026-10-10", "from=2026-10-12T00:00:00Z&to=2026-10-11T00:00:00Z",
		"page=1;page_size=2", "search=%ZZ",
	} {
		t.Run(query, func(t *testing.T) {
			s := &listServiceStub{}
			recorder := httptest.NewRecorder()
			listRouter(
				s,
			).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/supplies?"+query, http.NoBody))
			if recorder.Code != http.StatusUnprocessableEntity || s.calls != 0 {
				t.Fatalf("status %d, service calls %d; body %s", recorder.Code, s.calls, recorder.Body.String())
			}
			var response dto.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error.Code != "VALIDATION_ERROR" ||
				response.RequestID != recorder.Header().Get("X-Request-ID") {
				t.Fatal("validation did not use the common error format")
			}
			if _, err := uuid.Parse(response.RequestID); err != nil {
				t.Fatal("request ID is missing")
			}
		})
	}
}

func TestSupplyListParsesQueryAndDefaults(t *testing.T) {
	for _, query := range []url.Values{
		{},
		{
			"page": {"2"}, "page_size": {"3"}, "search": {"IN%_"}, "sort": {"-created_at"},
			"from": {"2026-10-10T15:00:00+03:00"}, "to": {"2026-10-11T15:00:00+03:00"},
		},
	} {
		s := &listServiceStub{}
		recorder := httptest.NewRecorder()
		listRouter(
			s,
		).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/supplies?"+query.Encode(), http.NoBody))
		if recorder.Code != http.StatusOK || s.calls != 1 {
			t.Fatalf("list failed: %s", recorder.Body.String())
		}
		var response supplydto.SupplyListResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Items == nil || response.Meta == nil {
			t.Fatal("empty list and metadata must not be null")
		}
		if len(query) == 0 {
			if response.Page != 1 || response.PageSize != 50 || s.filter.Search != nil {
				t.Fatal("incorrect defaults")
			}
			continue
		}
		if response.Page != 2 || response.PageSize != 3 || s.filter.Search == nil || *s.filter.Search != "IN%_" {
			t.Fatal("pagination or search was not forwarded")
		}
		if s.filter.DateFrom == nil || s.filter.DateTo == nil || s.filter.DateFrom.Hour() != 12 ||
			s.filter.DateTo.Hour() != 12 {
			t.Fatal("timezone-aware dates were not normalized")
		}
	}
}

func TestSupplyListDatabaseErrorIsSafe(t *testing.T) {
	s := &listServiceStub{err: errors.New("private SQL failure")}
	recorder := httptest.NewRecorder()
	listRouter(s).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/supplies", http.NoBody))
	var response dto.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusInternalServerError || response.Error.Code != "INTERNAL_ERROR" ||
		response.Error.Message == s.err.Error() {
		t.Fatal("database details escaped into the HTTP error")
	}
}

type detailServiceStub struct {
	Service
	calls int
	id    uuid.UUID
	now   time.Time
	err   error
}

func (s *detailServiceStub) GetSupplyByID(
	_ context.Context, id uuid.UUID, now time.Time,
) (*supplydto.SupplyDetailDTO, error) {
	s.calls++
	s.id, s.now = id, now
	return &supplydto.SupplyDetailDTO{}, s.err
}

func TestSupplyDetailRejectsInvalidParametersBeforeService(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"invalid", "00000000000000000000000000000001", id + "?unknown=1",
		id + "?sort=-created_at", id + "?search=%ZZ", id + "?x=1;x=2",
	} {
		t.Run(path, func(t *testing.T) {
			s := &detailServiceStub{}
			recorder := httptest.NewRecorder()
			listRouter(s).ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/api/v1/supplies/"+path, http.NoBody))
			var response dto.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusUnprocessableEntity || s.calls != 0 ||
				response.Error.Code != "VALIDATION_ERROR" || response.RequestID != recorder.Header().Get("X-Request-ID") {
				t.Fatalf("unexpected validation response: %s, calls %d", recorder.Body.String(), s.calls)
			}
		})
	}
}

func TestSupplyDetailUsesCommonErrorsAndOneServerTime(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"found", nil, http.StatusOK, ""},
		{"missing", apperrors.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"database failure", errors.New("private SQL failure"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &detailServiceStub{err: test.err}
			id := uuid.New()
			before := time.Now().UTC()
			recorder := httptest.NewRecorder()
			listRouter(s).ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/api/v1/supplies/"+id.String(), http.NoBody))
			if recorder.Code != test.status || s.calls != 1 || s.id != id || s.now.Before(before) ||
				s.now.After(time.Now()) || s.now.Location() != time.UTC {
				t.Fatalf("unexpected response/call: %s, calls %d", recorder.Body.String(), s.calls)
			}
			if test.err == nil {
				return
			}
			var response dto.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error.Code != test.code || strings.Contains(recorder.Body.String(), "private SQL") ||
				response.RequestID != recorder.Header().Get("X-Request-ID") {
				t.Fatal("incorrect common error response")
			}
		})
	}
}
