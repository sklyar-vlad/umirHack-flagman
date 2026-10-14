package deployment

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestComposeEndpointIsolation(t *testing.T) {
	for _, mode := range []string{"local", "vps"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(
				"docker",
				"compose",
				"--env-file",
				"/dev/null",
				"--profile",
				"tools",
				"-f",
				infra+"/compose."+mode+".yaml",
				"config",
				"--format",
				"json",
			)
			cmd.Env = append(
				os.Environ(),
				"COMPOSE_PROJECT_NAME=flagman-config-test",
				"POSTGRES_DB=test",
				"POSTGRES_USER=test",
				"POSTGRES_PASS=test-password",
				"POSTGRES_URL=postgres://test:test-password@db:5432/test",
				"FLAGMAN_DOMAIN=flagman.example",
				"ACME_EMAIL=test@example.com",
				"METRICS_TOKEN="+strings.Repeat("a", 64),
				"GRAFANA_ADMIN_PASSWORD=test-grafana-password",
				"GRAFANA_PG_PASSWORD=test-grafana-database-password",
			)
			data, err := cmd.Output()
			if err != nil {
				t.Fatal("compose config", err)
			}
			var cfg struct {
				Services map[string]struct {
					Ports []struct {
						HostIP string `json:"host_ip"`
						Target int
					}
					Networks  map[string]any
					Labels    map[string]string
					DependsOn map[string]struct{ Condition string } `json:"depends_on"`
				}
				Networks map[string]struct{ Internal bool }
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			if (mode == "vps" && !cfg.Networks["warehouse"].Internal) || len(cfg.Services["db"].Networks) != 1 {
				t.Fatal("database network is not isolated")
			}
			if _, ok := cfg.Services["db"].Networks["warehouse"]; !ok {
				t.Fatal("database on public network")
			}
			if cfg.Services["backend"].DependsOn["migrate"].Condition != "service_completed_successfully" ||
				cfg.Services["seed"].DependsOn["migrate"].Condition != "service_completed_successfully" {
				t.Fatal("application or seed can start before schema initialization")
			}
			for name, service := range cfg.Services {
				for _, port := range service.Ports {
					if port.Target != 80 && port.Target != 443 && port.HostIP != "127.0.0.1" {
						t.Fatal("public technical port", name, port)
					}
				}
			}
			if len(cfg.Services["backend"].Ports) != 0 {
				t.Fatal("backend port published")
			}
			if mode == "vps" {
				if len(cfg.Services["db"].Ports) != 0 || len(cfg.Services["prometheus"].Ports) != 0 {
					t.Fatal("VPS private service published")
				}
				if cfg.Services["frontend"].Labels["traefik.http.routers.frontend.tls.certresolver"] != "letsencrypt" {
					t.Fatal("VPS automatic HTTPS missing")
				}
			} else {
				if len(cfg.Services["db"].Ports) != 1 || cfg.Services["db"].Ports[0].Target != 5432 {
					t.Fatal("local database port missing")
				}
				if !strings.Contains(
					cfg.Services["backend"].Labels["traefik.http.routers.backend.rule"],
					"PathPrefix(`/api/`)",
				) {
					t.Fatal("public metrics route")
				}
			}
		})
	}
}
