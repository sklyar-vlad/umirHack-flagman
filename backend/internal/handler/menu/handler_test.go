package menu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
)

type fakeMenu struct{ calls int }

func (s *fakeMenu) ListDishes(context.Context, *model.DishFilter) (dto.DishListResponse, error) {
	s.calls++
	return dto.DishListResponse{Items: []dto.Dish{}}, nil
}

func (s *fakeMenu) StopList(context.Context, *model.DishFilter) (dto.DishListResponse, error) {
	s.calls++
	return dto.DishListResponse{Items: []dto.Dish{}}, nil
}

func (s *fakeMenu) GetDish(context.Context, uuid.UUID) (dto.Dish, error) {
	s.calls++
	return dto.Dish{}, nil
}

func TestMenuAuthorizationBeforeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"", "storekeeper", "admin", "manager", "order_operator", "unknown"} {
		for _, endpoint := range []string{"/dishes", "/stop-list", "/dishes/" + uuid.NewString()} {
			service := &fakeMenu{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if role != "" {
					c.Request = c.Request.WithContext(
						requestctx.WithActor(c.Request.Context(), model.Actor{ID: uuid.New(), Role: role}),
					)
				}
			})
			h := NewHandler(service)
			router.GET("/dishes", h.ListDishes)
			router.GET("/dishes/:id", h.GetDish)
			router.GET("/stop-list", h.StopList)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, http.NoBody))
			want := http.StatusOK
			if role == "" {
				want = http.StatusUnauthorized
			} else if role == "storekeeper" || role == "unknown" {
				want = http.StatusForbidden
			}
			if response.Code != want {
				t.Errorf("%s %s: status %d want %d", role, endpoint, response.Code, want)
			}
			if want != http.StatusOK && service.calls != 0 {
				t.Fatal("service called before authorization")
			}
		}
	}
}

func TestDishFilters(t *testing.T) {
	valid, _ := url.ParseQuery("page=2&page_size=3&search=суп&is_active=all&availability=unavailable&sort=name")
	filter, err := parseFilter(valid, false)
	if err != nil || filter.Page != 2 || filter.PageSize != 3 || filter.IsActive != nil ||
		filter.Availability != "unavailable" ||
		*filter.Search != "суп" {
		t.Fatalf("parse filter: %+v %v", filter, err)
	}
	for _, raw := range []string{"page=0", "page_size=101", "page=9223372036854775807&page_size=100", "sort=id", "availability=bad", "is_active=bad", "search=a&search=b", "unknown=1"} {
		query, _ := url.ParseQuery(raw)
		if _, err := parseFilter(query, false); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"is_active=all", "availability=all"} {
		query, _ := url.ParseQuery(raw)
		if _, err := parseFilter(query, true); err == nil {
			t.Errorf("stop list accepted %q", raw)
		}
	}
}
