package telemetry

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const labelWorker = "worker"

type Metrics struct {
	Registry       *prometheus.Registry
	HTTP           *prometheus.CounterVec
	Duration       *prometheus.HistogramVec
	Refusals       *prometheus.CounterVec
	Conflicts      *prometheus.CounterVec
	Retries        *prometheus.CounterVec
	WorkerDuration *prometheus.HistogramVec
	WorkerErrors   *prometheus.CounterVec
	WorkerSuccess  *prometheus.GaugeVec
}

var Default = New()

func New() *Metrics {
	m := &Metrics{Registry: prometheus.NewRegistry()}
	m.HTTP = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flagman_http_requests_total",
			Help: "Completed HTTP requests by template and status.",
		},
		[]string{"method", "route", "status"},
	)
	m.Duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "flagman_http_request_duration_seconds",
			Help:    "Completed request duration including stream lifetime.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
	m.Refusals = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flagman_domain_refusals_total",
			Help: "New committed domain refusals, excluding idempotency replay.",
		},
		[]string{"reason"},
	)
	m.Conflicts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flagman_transaction_conflicts_total",
			Help: "Transactions failing with retryable serialization/deadlock conflicts; no automatic request retry.",
		},
		[]string{"reason"},
	)
	m.Retries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flagman_transaction_retries_total",
			Help: "Actual worker retry passes following a failed transactional pass.",
		},
		[]string{labelWorker},
	)
	m.WorkerDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "flagman_worker_pass_duration_seconds",
			Help:    "Duration of background transactional passes.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelWorker},
	)
	m.WorkerErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flagman_worker_errors_total",
			Help: "Failed background passes, excluding lifecycle cancellation.",
		},
		[]string{labelWorker},
	)
	m.WorkerSuccess = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "flagman_worker_last_success_timestamp_seconds",
			Help: "Last successful pass; zero before first success.",
		},
		[]string{labelWorker},
	)
	m.Registry.MustRegister(
		m.HTTP,
		m.Duration,
		m.Refusals,
		m.Conflicts,
		m.Retries,
		m.WorkerDuration,
		m.WorkerErrors,
		m.WorkerSuccess,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	for _, w := range []string{"outbox", "expiration"} {
		m.Retries.WithLabelValues(w)
		m.WorkerErrors.WithLabelValues(w)
		m.WorkerSuccess.WithLabelValues(w)
	}
	for _, r := range []string{"serialization", "deadlock"} {
		m.Conflicts.WithLabelValues(r)
	}
	return m
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD":
		default:
			method = "OTHER"
		}
		status := c.Writer.Status()
		if status < 100 || status > 599 {
			status = 500
		}
		m.HTTP.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		m.Duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

func (m *Metrics) Refusal(body []byte) {
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return
	}
	switch response.Error.Code {
	case "INSUFFICIENT_STOCK",
		"DISH_UNAVAILABLE",
		"RECIPE_CHANGED",
		"INSUFFICIENT_BATCH_STOCK",
		"PRODUCT_INACTIVE",
		"SHELF_LIFE_REQUIRED":
		m.Refusals.WithLabelValues(response.Error.Code).Inc()
	}
}

func (m *Metrics) Conflict(code string) {
	switch code {
	case "40001":
		m.Conflicts.WithLabelValues("serialization").Inc()
	case "40P01":
		m.Conflicts.WithLabelValues("deadlock").Inc()
	}
}

func (m *Metrics) Worker(worker string, start time.Time, err error, canceled, retry bool) {
	if worker != "outbox" && worker != "expiration" {
		return
	}
	m.WorkerDuration.WithLabelValues(worker).Observe(time.Since(start).Seconds())
	if retry {
		m.Retries.WithLabelValues(worker).Inc()
	}
	if err == nil {
		m.WorkerSuccess.WithLabelValues(worker).SetToCurrentTime()
	} else if !canceled {
		m.WorkerErrors.WithLabelValues(worker).Inc()
	}
}

func (m *Metrics) Handler(token string) http.Handler {
	handler := promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Timeout: 4 * time.Second, MaxRequestsInFlight: 2})
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if token == "" {
			http.NotFound(w, r)
			return
		}
		values := r.Header.Values("Authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		supplied := sha256.Sum256([]byte(strings.TrimPrefix(values[0], "Bearer ")))
		if subtle.ConstantTimeCompare(supplied[:], expected[:]) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

type (
	Snapshot           func(context.Context) (pending, age, expiryAge float64, err error)
	inventoryCollector struct {
		snapshot                 Snapshot
		pending, age, expiry, up *prometheus.Desc
	}
)

func (m *Metrics) RegisterSnapshot(snapshot Snapshot) {
	m.Registry.MustRegister(&inventoryCollector{
		snapshot: snapshot,
		pending:  prometheus.NewDesc("flagman_outbox_pending_events", "Committed unpublished events.", nil, nil),
		age: prometheus.NewDesc(
			"flagman_outbox_oldest_pending_age_seconds",
			"Age of oldest committed unpublished event; zero when empty.",
			nil,
			nil,
		),
		expiry: prometheus.NewDesc(
			"flagman_expiration_oldest_overdue_age_seconds",
			"Age of oldest positive expired batch when automatic write-off is enabled.",
			nil,
			nil,
		),
		up: prometheus.NewDesc(
			"flagman_metrics_database_up",
			"Whether the bounded metrics snapshot succeeded.",
			nil,
			nil,
		),
	})
}

func (c *inventoryCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.pending, c.age, c.expiry, c.up} {
		ch <- d
	}
}

func (c *inventoryCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pending, age, expiry, err := c.snapshot(ctx)
	up := float64(1)
	if err != nil {
		up = 0
	} else {
		ch <- prometheus.MustNewConstMetric(c.pending, prometheus.GaugeValue, pending)
		ch <- prometheus.MustNewConstMetric(c.age, prometheus.GaugeValue, age)
		ch <- prometheus.MustNewConstMetric(c.expiry, prometheus.GaugeValue, expiry)
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, up)
}
