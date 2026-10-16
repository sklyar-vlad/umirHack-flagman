package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalRuntimeConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"POSTGRES_URL", "POSTGRES_URL_FILE", "METRICS_TOKEN", "METRICS_TOKEN_FILE", "FRONTEND_ORIGIN"} {
		t.Setenv(key, "")
	}
	t.Setenv("ENV", "production")
	if _, err := NewConfig(); err == nil {
		t.Fatal("production without database accepted")
	}
	dir := t.TempDir()
	database := filepath.Join(dir, "database")
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(database, []byte("postgres://test:test@db:5432/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte(strings.Repeat("x", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POSTGRES_URL_FILE", database)
	t.Setenv("METRICS_TOKEN_FILE", token)
	for _, origin := range []string{"", "http://flagman.example"} {
		t.Setenv("FRONTEND_ORIGIN", origin)
		if _, err := NewConfig(); err == nil {
			t.Fatal("insecure production origin accepted")
		}
	}
	t.Setenv("FRONTEND_ORIGIN", "https://flagman.example")
	cfg, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.PostgresURL != "postgres://test:test@db:5432/test" || cfg.MetricsToken != strings.Repeat("x", 32) ||
		cfg.Auth.FrontendOrigin != "https://flagman.example" {
		t.Fatal("external configuration mismatch")
	}
	t.Setenv("POSTGRES_URL", "secret-that-must-not-be-logged")
	if _, err := NewConfig(); err == nil || strings.Contains(err.Error(), "secret-that") {
		t.Fatal("ambiguous source accepted or leaked")
	}
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("POSTGRES_URL_FILE", filepath.Join(dir, "missing-secret-path"))
	if _, err := NewConfig(); err == nil || strings.Contains(err.Error(), "missing-secret-path") {
		t.Fatal("missing file accepted or path leaked")
	}
	t.Setenv("POSTGRES_URL_FILE", database)
	for _, value := range []string{"", "short"} {
		if err := os.WriteFile(token, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewConfig(); err == nil {
			t.Fatal("invalid metrics secret accepted")
		}
	}
}

func TestExplicitLocalHTTPConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"POSTGRES_URL", "POSTGRES_URL_FILE", "METRICS_TOKEN", "METRICS_TOKEN_FILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("ENV", "development")
	t.Setenv("FRONTEND_ORIGIN", "http://flagman.localhost")
	cfg, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.FrontendOrigin != "http://flagman.localhost" {
		t.Fatal("local origin changed")
	}
	if err := os.WriteFile(".env", []byte("! invalid environment"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfig(); err == nil {
		t.Fatal("malformed dotenv ignored")
	}
}
