package events

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func proxyPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func TestCurlUnbufferedThroughNginxAndTraefik(t *testing.T) {
	if os.Getenv("FLAGMAN_TEST_SSE_PROXIES") != "1" {
		t.Skip("set FLAGMAN_TEST_SSE_PROXIES=1 with local nginx/frontend and Traefik images")
	}
	f := newFixture(t)
	server, s, token, _ := f.stream(t, "admin")
	dir := t.TempDir()
	port := proxyPort(t)
	raw, err := os.ReadFile("../../../../infrastructure/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	nginx := strings.ReplaceAll(string(raw), "listen 80;", fmt.Sprintf("listen 127.0.0.1:%d;", port))
	nginx = strings.ReplaceAll(nginx, "http://backend:8080", server.URL)
	nginx = "pid /tmp/nginx.pid; error_log stderr warn; events {} http { access_log off; client_body_temp_path /tmp/client; proxy_temp_path /tmp/proxy; fastcgi_temp_path /tmp/fastcgi; uwsgi_temp_path /tmp/uwsgi; scgi_temp_path /tmp/scgi; " + nginx + " }"
	if err := os.WriteFile(filepath.Join(dir, "nginx.conf"), []byte(nginx), 0o600); err != nil {
		t.Fatal(err)
	}
	nginxImage := os.Getenv("FLAGMAN_TEST_NGINX_IMAGE")
	if nginxImage == "" {
		nginxImage = "flagman-frontend:latest"
	}
	startProxy(t, dir, nginxImage, "nginx", "-c", "/test/nginx.conf", "-g", "daemon off;")
	nginxURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitProxy(t, nginxURL)

	raw, err = os.ReadFile("../../../../infrastructure/compose.local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "--entrypoints.web.transport.respondingtimeouts.writetimeout=0s") ||
		!strings.Contains(string(raw), "responseforwarding.flushinterval: -1ms") {
		t.Fatal("Compose stream options missing")
	}
	dynamic := fmt.Sprintf(`http:
  routers:
    api:
      rule: 'PathPrefix("/api/")'
      service: api
  services:
    api:
      loadBalancer:
        responseForwarding:
          flushInterval: -1ms
        servers:
          - url: "%s"
`, nginxURL)

	if err := os.WriteFile(filepath.Join(dir, "dynamic.yaml"), []byte(dynamic), 0o600); err != nil {
		t.Fatal(err)
	}
	traefikPort := proxyPort(t)
	startProxy(
		t,
		dir,
		"traefik:v3.7",
		"traefik",
		"--providers.file.filename=/test/dynamic.yaml",
		fmt.Sprintf("--entrypoints.web.address=127.0.0.1:%d", traefikPort),
		"--entrypoints.web.transport.respondingtimeouts.writetimeout=0s",
		"--log.level=ERROR",
	)
	traefikURL := fmt.Sprintf("http://127.0.0.1:%d", traefikPort)
	waitProxy(t, traefikURL)
	for _, url := range []string{server.URL, nginxURL, traefikURL} {
		t.Run(url, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), "curl.conf")
			if err := os.WriteFile(
				config,
				[]byte(fmt.Sprintf("cookie = \"flagman_session=%s\"\nurl = \"%s/api/v1/events\"\n", token, url)),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "curl", "-N", "-sS", "--config", config)
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = io.Discard
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); cmd.Wait() }()
			r := bufio.NewReader(pipe)
			line, err := r.ReadString('\n')
			if err != nil || line != ": connected\n" {
				t.Fatal("buffered initial frame", line, err)
			}

			timer := time.NewTimer(200 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
			cursor := f.append(
				t,
				s,
				"inventory.changed",
				&model.OutboxPayload{Topics: []model.EventTopic{model.TopicInventory}},
			)
			if got := next(t, r); got.ID != cursor {
				t.Fatal(got, cursor)
			}
		})
	}
}

func startProxy(t *testing.T, dir, image, entry string, args ...string) {
	t.Helper()
	name := "flagman-sse-test-" + uuid.NewString()
	command := []string{
		"run",
		"--rm",
		"--pull=never",
		"--name",
		name,
		"--network=host",
		"--read-only",
		"--tmpfs",
		"/tmp",
		"--cap-drop=ALL",
		"--user",
		fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v",
		dir + ":/test:ro",
		"--entrypoint",
		entry,
		image,
	}
	command = append(command, args...)
	cmd := exec.Command("docker", command...)
	logFile, err := os.CreateTemp(t.TempDir(), "proxy-log")
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		exec.CommandContext(ctx, "docker", "stop", "-t", "1", name).Run()
		cmd.Wait()
		logFile.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logFile.Name())
			t.Log(string(data))
		}
	})
}

func waitProxy(t *testing.T, url string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("proxy did not start", url)
		case <-tick.C:
			conn, err := net.DialTimeout("tcp", strings.TrimPrefix(url, "http://"), 50*time.Millisecond)
			if err == nil {
				conn.Close()
				return
			}
		}
	}
}
