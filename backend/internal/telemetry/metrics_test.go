package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	r.Header.Set("Authorization", "Bearer test-secret")
	m.Handler("test-secret").ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestBoundedRouteMethodAndConcurrentMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New()
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/products/:id", func(c *gin.Context) { c.Status(500) })
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			for _, req := range []*http.Request{httptest.NewRequest(http.MethodGet, "/products/private-id?secret=token", http.NoBody), httptest.NewRequest("ARBITRARY_SECRET", "/not-found-secret", http.NoBody)} {
				r.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
	wg.Wait()
	if n := testutil.ToFloat64(m.HTTP.WithLabelValues("GET", "/products/:id", "500")); n != 100 {
		t.Fatal(n)
	}
	s := scrape(t, m)
	for _, secret := range []string{"private-id", "not-found-secret", "token", "ARBITRARY_SECRET"} {
		if strings.Contains(s, secret) {
			t.Fatal("unbounded label", secret)
		}
	}
	if !strings.Contains(s, `route="unmatched"`) || !strings.Contains(s, `method="OTHER"`) {
		t.Fatal("missing bounded fallback")
	}
}

func TestDomainConflictWorkerAndSecretLabels(t *testing.T) {
	m := New()
	m.Refusal([]byte(`{"error":{"code":"INSUFFICIENT_STOCK","details":{"id":"secret"}}}`))
	m.Refusal([]byte(`{"error":{"code":"secret"}}`))
	m.Conflict("40001")
	m.Conflict("40P01")
	m.Conflict("secret")
	m.Worker("outbox", time.Now(), errors.New("secret"), false, false)
	m.Worker("outbox", time.Now(), nil, false, true)
	m.Worker("expiration", time.Now(), context.Canceled, true, false)
	m.Worker("secret", time.Now(), nil, false, false)
	s := scrape(t, m)
	if strings.Contains(s, "secret") {
		t.Fatal("secret leaked")
	}
	if testutil.ToFloat64(m.Refusals.WithLabelValues("INSUFFICIENT_STOCK")) != 1 ||
		testutil.ToFloat64(m.Retries.WithLabelValues("outbox")) != 1 ||
		testutil.ToFloat64(m.WorkerErrors.WithLabelValues("expiration")) != 0 ||
		testutil.ToFloat64(m.WorkerSuccess.WithLabelValues("outbox")) == 0 {
		t.Fatal("counter semantics")
	}
}

func TestAuthenticationPrecedesSnapshotAndFailureDoesNotReportZeroLag(t *testing.T) {
	m := New()
	calls := 0
	m.RegisterSnapshot(func(ctx context.Context) (float64, float64, float64, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("unbounded query")
		}
		return 0, 0, 0, errors.New("secret")
	})

	for _, token := range []string{"", "Bearer wrong", "bearer test-secret"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
		r.Header.Set("Authorization", token)
		m.Handler("test-secret").ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	m.Handler("").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	if w.Code != 404 || calls != 0 {
		t.Fatal("disabled or unauthorized snapshot queried")
	}
	s := scrape(t, m)
	if !strings.Contains(s, "flagman_metrics_database_up 0") || strings.Contains(s, "flagman_outbox_pending_events") ||
		strings.Contains(s, "secret") {
		t.Fatal("failed query masked")
	}
}
