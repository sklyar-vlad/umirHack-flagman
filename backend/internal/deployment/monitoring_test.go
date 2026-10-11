package deployment

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"gopkg.in/yaml.v3"
)

const infra = "../../../infrastructure"

func TestDashboardSourcesAndUnitGrouping(t *testing.T) {
	data, err := os.ReadFile(infra + "/monitoring/grafana/dashboards/flagman.json")
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Panels []struct {
			Title   string
			Targets []struct{ Expr, RawSql string }
		}
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Panels) < 10 {
		t.Fatal("missing dashboard panels")
	}
	sqlCount := 0
	for _, p := range d.Panels {
		for _, q := range p.Targets {
			if q.RawSql != "" {
				sqlCount++
				if !strings.Contains(q.RawSql, "monitoring_") || strings.Contains(q.RawSql, "users") {
					t.Fatal("unsafe data source", q.RawSql)
				}
				if strings.Contains(q.RawSql, "sum(quantity)") &&
					!strings.Contains(q.RawSql, "GROUP BY product_id,unit,reason") {
					t.Fatal("mixed units")
				}
			}
		}
	}
	if sqlCount != 4 {
		t.Fatal("missing warehouse panels", sqlCount)
	}
	for _, name := range []string{"monitoring/prometheus.yaml", "monitoring/grafana/provisioning/datasources/flagman.yaml", "monitoring/grafana/provisioning/dashboards/flagman.yaml", "compose.monitoring.yaml"} {
		b, err := os.ReadFile(infra + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var parsed any
		if err := yaml.Unmarshal(b, &parsed); err != nil {
			t.Fatal(name, err)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func wait(t *testing.T, check func() bool) {
	t.Helper()
	until := time.Now().Add(30 * time.Second)
	for time.Now().Before(until) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("container not ready")
}

func container(t *testing.T, args ...string) {
	t.Helper()
	name := "flagman-monitoring-test-" + uuid.NewString()
	cmd := exec.Command(
		"docker",
		append([]string{"run", "-d", "--name", name, "--network=host", "--read-only", "--cap-drop=ALL"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test container failed: %v %s", err, out)
	}
	t.Cleanup(func() {
		if t.Failed() {
			out, _ := exec.Command("docker", "logs", name).CombinedOutput()
			if len(out) > 5000 {
				out = out[len(out)-5000:]
			}
			t.Log(string(out))
		}
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
}

func TestPrometheusScrapesProtectedBackend(t *testing.T) {
	if os.Getenv("FLAGMAN_TEST_MONITORING") != "1" {
		t.Skip("set FLAGMAN_TEST_MONITORING=1 for container tests")
	}
	m := telemetry.New()
	m.HTTP.WithLabelValues("GET", "/ready", "200").Inc()
	token := uuid.NewString()
	api := httptest.NewServer(m.Handler(token))
	defer api.Close()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metrics_token"), []byte(token), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(infra + "/monitoring/prometheus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := strings.ReplaceAll(string(data), "backend:8080", strings.TrimPrefix(api.URL, "http://"))
	cfg = strings.ReplaceAll(cfg, "15s", "1s")
	cfg = strings.ReplaceAll(cfg, "scrape_timeout: 5s", "scrape_timeout: 1s")
	if err := os.WriteFile(filepath.Join(dir, "prometheus.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	container(
		t,
		"--tmpfs",
		"/prometheus:uid=65534,gid=65534",
		"-v",
		dir+":/test:ro",
		"-v",
		filepath.Join(dir, "metrics_token")+":/run/secrets/metrics_token:ro",
		"prom/prometheus:v3.5.0",
		"--config.file=/test/prometheus.yaml",
		"--web.listen-address=127.0.0.1:"+strconv.Itoa(port),
	)
	client := &http.Client{Timeout: time.Second}
	query := func() bool {
		r, err := client.Get(
			"http://127.0.0.1:" + strconv.Itoa(
				port,
			) + "/api/v1/query?query=" + url.QueryEscape(
				`flagman_http_requests_total{route="/ready"}`,
			),
		)
		if err != nil {
			return false
		}
		defer r.Body.Close()
		var body struct {
			Status string
			Data   struct{ Result []struct{ Value []any } }
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			return false
		}
		return body.Status == "success" && len(body.Data.Result) == 1 && body.Data.Result[0].Value[1] == "1"
	}
	wait(t, query)
	// A non-authenticated request never exposes metrics.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, api.URL, http.NoBody)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("scrape auth not required")
	}
}
