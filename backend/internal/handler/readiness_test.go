package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestReadinessIsSafeAndIndependentOfLiveness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fail := range []bool{false, true} {
		r := gin.New()
		r.Use(RateLimit(1, 1))
		r.GET("/api/v1/ready", Readiness(func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second {
				t.Fatal("missing bound")
			}
			if fail {
				return errors.New("postgres password secret")
			}
			return nil
		}))
		h := NewHandler(nil, nil, zap.NewNop())
		r.GET("/api/v1/healthcheck", h.Healthcheck)
		for range 3 {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ready", http.NoBody))
			want := 200
			if fail {
				want = 503
			}
			if w.Code != want || strings.Contains(w.Body.String(), "secret") ||
				w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthcheck", http.NoBody))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"ok"`) {
			t.Fatal("liveness changed")
		}
	}
}
