package deployment

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestComposeEndpointIsolation(t *testing.T) {
	cmd := exec.Command("docker", "compose", "-f", infra+"/compose.yaml", "-f", infra+"/compose.monitoring.yaml", "config", "--format", "json")
	cmd.Env = append(os.Environ(), "POSTGRES_DB=test", "POSTGRES_USER=test", "POSTGRES_PASS=test", "FLAGMAN_SECRETS_DIR=/tmp/flagman-test-secrets")
	b, err := cmd.Output()
	if err != nil {
		t.Fatal("compose config", err)
	}
	var cfg struct {
		Services map[string]struct {
			Ports []struct {
				HostIP    string `json:"host_ip"`
				Published string `json:"published"`
				Target    int
			}
			Networks map[string]any
			Labels   map[string]string
		}
		Networks map[string]struct{ Internal bool }
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"db", "backend", "prometheus"} {
		if len(cfg.Services[name].Ports) != 0 {
			t.Fatal("published private service", name)
		}
	}
	for _, name := range []string{"grafana", "traefik"} {
		for _, port := range cfg.Services[name].Ports {
			if port.Target != 80 && port.HostIP != "127.0.0.1" {
				t.Fatal("public technical port", name, port)
			}
		}
	}
	if !cfg.Networks["warehouse"].Internal || len(cfg.Services["db"].Networks) != 1 {
		t.Fatal("database network is not isolated")
	}
	if _, ok := cfg.Services["db"].Networks["warehouse"]; !ok {
		t.Fatal("database on public network")
	}
	if cfg.Services["traefik"].Labels["traefik.http.routers.traefik.entrypoints"] != "admin" {
		t.Fatal("dashboard on public entrypoint")
	}
	if !strings.Contains(cfg.Services["backend"].Labels["traefik.http.routers.backend.rule"], "PathPrefix(`/api/`)") {
		t.Fatal("public metrics route")
	}
}
