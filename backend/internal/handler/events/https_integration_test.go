package events

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func TestHTTPSDeploymentCookiesCORSAndStreaming(t *testing.T) {
	if os.Getenv("FLAGMAN_TEST_HTTPS") != "1" {
		t.Skip("set FLAGMAN_TEST_HTTPS=1")
	}
	f := newFixture(t)
	server, service, _, _ := f.stream(t, "admin")
	password := "correct HTTPS test password"
	hash, err := authservice.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE users SET password_hash=$1 WHERE email='user@example.test'`, hash)
	dir := t.TempDir()
	// A local trusted certificate verifies SNI and the handshake without -k.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "flagman.example"},
		DNSNames:              []string{"flagman.example"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(48 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("fullchain.pem", certPEM)
	write(
		"privkey.pem",
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	)
	nginxPort := proxyPort(t)
	raw, err := os.ReadFile("../../../../infrastructure/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	nginx := strings.ReplaceAll(string(raw), "listen 80;", fmt.Sprintf("listen 127.0.0.1:%d;", nginxPort))
	nginx = strings.ReplaceAll(nginx, "http://backend:8080", server.URL)
	write(
		"nginx.conf",
		[]byte(
			"pid /tmp/nginx.pid; error_log stderr warn; events {} http { access_log off; client_body_temp_path /tmp/client; proxy_temp_path /tmp/proxy; fastcgi_temp_path /tmp/fastcgi; uwsgi_temp_path /tmp/uwsgi; scgi_temp_path /tmp/scgi; "+nginx+" }",
		),
	)
	startProxy(t, dir, "flagman-frontend:latest", "nginx", "-c", "/test/nginx.conf", "-g", "daemon off;")
	waitProxy(t, fmt.Sprintf("http://127.0.0.1:%d", nginxPort))
	// Render the actual standalone deployment. Only ports/provider change for isolation.
	command := exec.Command(
		"docker",
		"compose",
		"-f",
		"../../../../infrastructure/compose.vps.yaml",
		"config",
		"--format",
		"json",
	)
	command.Env = append(
		os.Environ(),
		"POSTGRES_DB=test",
		"POSTGRES_USER=test",
		"FLAGMAN_DOMAIN=flagman.example",
		"POSTGRES_PASS=isolated-test-password",
		"POSTGRES_URL=postgres://test:isolated-test-password@db:5432/test",
		"ACME_EMAIL=test@example.com",
		"METRICS_TOKEN="+strings.Repeat("a", 64),
		"GRAFANA_ADMIN_PASSWORD=isolated-grafana-password",
		"GRAFANA_PG_PASSWORD=isolated-grafana-database-password",
	)
	b, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Services map[string]struct {
			Command             []string
			Labels, Environment map[string]string
			Ports               []struct {
				HostIP string `json:"host_ip"`
				Target int
			}
		}
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Services["backend"].Environment["FRONTEND_ORIGIN"] != "https://flagman.example" {
		t.Fatal("wrong production origin")
	}
	for name, s := range cfg.Services {
		for _, p := range s.Ports {
			if p.Target != 80 && p.Target != 443 && p.HostIP != "127.0.0.1" {
				t.Fatal("public private endpoint", name)
			}
		}
	}
	rule := cfg.Services["frontend"].Labels["traefik.http.routers.frontend.rule"]
	tlsConfig := "\ntls:\n  certificates:\n    - certFile: /test/fullchain.pem\n      keyFile: /test/privkey.pem\n"
	dynamic := fmt.Sprintf(`http:
  routers:
    frontend:
      rule: '%s'
      entryPoints: [websecure]
      tls: {}
      service: frontend
    admin:
      rule: 'PathPrefix(`+"`/dashboard/`"+`) || PathPrefix(`+"`/api/`"+`)'
      entryPoints: [admin]
      service: api@internal
  services:
    frontend:
      loadBalancer:
        responseForwarding:
          flushInterval: -1ms
        servers:
          - url: "http://127.0.0.1:%d"
`, rule, nginxPort) + tlsConfig
	write("dynamic.yaml", []byte(dynamic))
	httpPort, tlsPort, adminPort := proxyPort(t), proxyPort(t), proxyPort(t)
	args := []string{
		"--api.dashboard=true",
		"--providers.file.filename=/test/dynamic.yaml",
		fmt.Sprintf("--entrypoints.admin.address=127.0.0.1:%d", adminPort),
	}
	for _, arg := range cfg.Services["traefik"].Command {
		switch {
		case strings.HasPrefix(arg, "--providers.docker="):
			arg = "--providers.docker=false"
		case strings.HasPrefix(arg, "--providers.docker.") || strings.HasPrefix(arg, "--certificatesresolvers.") || arg == "--api.insecure=true":
			continue
		case strings.HasPrefix(arg, "--providers.file.filename="):
			arg = "--providers.file.filename=/test/dynamic.yaml"
		case strings.HasPrefix(arg, "--entrypoints.web.address="):
			arg = fmt.Sprintf("--entrypoints.web.address=127.0.0.1:%d", httpPort)
		case strings.HasPrefix(arg, "--entrypoints.websecure.address="):
			arg = fmt.Sprintf("--entrypoints.websecure.address=127.0.0.1:%d", tlsPort)
		case strings.HasPrefix(arg, "--entrypoints.admin.address="):
			arg = fmt.Sprintf("--entrypoints.admin.address=127.0.0.1:%d", adminPort)
		}
		args = append(args, arg)
	}
	startProxy(t, dir, "traefik:v3.7", "traefik", args...)
	waitProxy(t, fmt.Sprintf("http://127.0.0.1:%d", tlsPort))
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "flagman.example", MinVersion: tls.VersionTLS12},
	}
	defer transport.CloseIdleConnections()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Transport:     transport,
		Jar:           jar,
		Timeout:       8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	base := fmt.Sprintf("https://127.0.0.1:%d", tlsPort)
	request := func(method, path, body, origin, csrf string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "flagman.example"
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v1/auth/me", httpPort), http.NoBody)
	req.Host = "flagman.example"
	var res *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err = client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusMovedPermanently || res.StatusCode == http.StatusPermanentRedirect {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if (res.StatusCode != http.StatusMovedPermanently && res.StatusCode != http.StatusPermanentRedirect) ||
		!strings.HasPrefix(res.Header.Get("Location"), "https://") {
		t.Fatal("HTTP did not redirect to TLS", res.StatusCode)
	}
	// The redirect router can appear before file-provider TLS configuration.
	// Wait for the actual trusted handshake and application route, not just TCP.
	readyReq, _ := http.NewRequest(http.MethodGet, base+"/api/v1/auth/me", http.NoBody)
	readyReq.Host = "flagman.example"
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err = client.Do(readyReq)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusUnauthorized {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatal("HTTPS application did not become ready", err)
	}
	for _, origin := range []string{"http://flagman.example", "https://flagman.example.evil", "https://flagman.example:443"} {
		res = request(
			"POST",
			"/api/v1/auth/login",
			`{"email":"user@example.test","password":"`+password+`"}`,
			origin,
			"",
		)
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden || res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("wrong origin accepted", origin, res.StatusCode)
		}
	}
	res = request(
		"POST",
		"/api/v1/auth/login",
		`{"email":"user@example.test","password":"`+password+`"}`,
		"https://flagman.example",
		"",
	)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatal("login failed", res.StatusCode)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Domain != "" ||
		cookies[0].SameSite != http.SameSiteLaxMode ||
		cookies[0].Path != "/api/v1" {
		t.Fatal("unsafe proxied session cookie")
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "https://flagman.example" ||
		res.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("CORS mismatch")
	}
	var login dto.AuthResponse
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res = request("GET", "/api/v1/auth/me", "", "https://flagman.example", "")
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatal("cookie jar session failed")
	}
	for _, path := range []string{"/metrics", "/dashboard/", "/api/rawdata"} {
		res = request("GET", path, "", "", "")
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "flagman_http_requests_total") || strings.Contains(string(b), "\"routers\"") ||
			strings.Contains(string(b), "Traefik") {
			t.Fatal("technical endpoint exposed", path)
		}
	}
	// The admin API is available solely on its separate loopback entrypoint.
	res, err = client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/rawdata", adminPort))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatal("admin route unavailable")
	}
	res = request("GET", "/api/v1/events", "", "", "")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("SSE failed", res.StatusCode)
	}
	reader := bufio.NewReader(res.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatal("buffered first SSE frame", err)
	}
	time.Sleep(200 * time.Millisecond) // Beyond the backend's ordinary response deadline.
	cursor := f.append(
		t,
		service,
		"inventory.changed",
		&model.OutboxPayload{Topics: []model.EventTopic{model.TopicInventory}},
	)
	if event := next(t, reader); event.ID != cursor {
		t.Fatal("TLS SSE event mismatch", event.ID, cursor)
	}
	res.Body.Close()
	res = request("POST", "/api/v1/auth/logout", "", "https://flagman.example", "")
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatal("missing CSRF accepted")
	}
	res = request("POST", "/api/v1/auth/logout", "", "https://flagman.example", login.CSRFToken)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatal("safe logout failed")
	}
	res = request("GET", "/api/v1/auth/me", "", "", "")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatal("revoked cookie accepted")
	}
	// Unknown SNI must be refused, rather than serving an unrelated certificate.
	badTransport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "unknown.example"}}
	defer badTransport.CloseIdleConnections()
	bad := &http.Client{Transport: badTransport, Timeout: time.Second}
	if res, err := bad.Get(base + "/"); err == nil {
		res.Body.Close()
		t.Fatal("unknown TLS host accepted")
	}
}
