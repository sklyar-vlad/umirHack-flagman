package database

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBackupRestore(t *testing.T) {
	if os.Getenv("FLAGMAN_TEST_BACKUP") != "1" {
		t.Skip("set FLAGMAN_TEST_BACKUP=1")
	}
	// Reuse the dedicated-local validation, never the application DSN.
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	source := "flagman_backup_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target := "flagman_restore_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg := f.pool.Config().Copy()
	delete(cfg.ConnConfig.RuntimeParams, "search_path")
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{source}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, name := range []string{target, source} {
			if _, err := f.pool.Exec(clean, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
				t.Error(err)
			}
		}
	})
	cfg.ConnConfig.Database = source
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	scripts, err := filepath.Abs("../../infrastructure/database")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	migrationDir, err := filepath.Abs("migrations")
	if err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dir, "postgres_url")
	c := cfg.ConnConfig
	sourceURL := (&url.URL{Scheme: "postgres", User: url.UserPassword(c.User, c.Password), Host: net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port))), Path: "/" + source, RawQuery: "sslmode=disable"}).String()
	if err := os.WriteFile(secretPath, []byte(sourceURL), 0600); err != nil {
		t.Fatal(err)
	}
	migrate := exec.CommandContext(ctx, "docker", "run", "--rm", "--pull=never", "--network=host", "--read-only", "--user", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()), "-v", scripts+":/scripts:ro", "-v", migrationDir+":/migrations:ro", "-v", secretPath+":/run/secrets/postgres_url:ro", "--entrypoint", "sh", "gomicro/goose:latest", "/scripts/migrate.sh")
	if out, err := migrate.CombinedOutput(); err != nil {
		t.Fatalf("isolated migration tool failed: %v %s", err, out)
	}
	if err := CheckReady(ctx, pool); err != nil {
		t.Fatal("migrated source readiness", err)
	}
	seedMonitoring(t, &fixture{ctx: ctx, pool: pool})

	run := func(script string, args ...string) error {
		cmd := exec.CommandContext(ctx, "docker", append([]string{"run", "--rm", "--pull=never", "--network=host", "--read-only", "--cap-drop=ALL", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "-v", scripts + ":/scripts:ro", "-v", dir + ":/backup", "-e", "PGHOST", "-e", "PGPORT", "-e", "PGUSER", "-e", "PGPASSWORD", "-e", "PGDATABASE", "postgres:18-alpine", "sh", "/scripts/" + script}, args...)...)
		c := cfg.ConnConfig
		cmd.Env = append(os.Environ(), "PGHOST="+c.Host, "PGPORT="+strconv.Itoa(int(c.Port)), "PGUSER="+c.User, "PGPASSWORD="+c.Password, "PGDATABASE="+source)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("script %s: %s", script, out)
		}
		return err
	}
	if err := run("backup.sh", "/backup/warehouse.dump"); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(dir, "warehouse.dump"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run("backup.sh", "/backup/warehouse.dump"); err == nil {
		t.Fatal("overwrote archive")
	}
	current, _ := os.ReadFile(filepath.Join(dir, "warehouse.dump"))
	if string(original) != string(current) {
		t.Fatal("archive changed")
	}
	if err := run("restore-test.sh", "/backup/warehouse.dump", source); err == nil {
		t.Fatal("allowed source restore")
	}
	if err := run("restore-test.sh", "/backup/warehouse.dump", target); err != nil {
		t.Fatal(err)
	}
	if err := run("restore-test.sh", "/backup/warehouse.dump", target); err == nil {
		t.Fatal("overwrote existing database")
	}
	cfg.ConnConfig.Database = target
	restored, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := CheckReady(ctx, restored); err != nil {
		t.Fatal("restored readiness", err)
	}
	// Compare every stored fact and ledger, not just row counts.
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	names = append(names, "monitoring_stock", "monitoring_problem_batches", "monitoring_stop_list", "monitoring_losses")
	for _, name := range names {
		query := `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM ` + pgx.Identifier{name}.Sanitize() + ` r`
		var before, after string
		if err := pool.QueryRow(ctx, query).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if err := restored.QueryRow(ctx, query).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatalf("restored facts differ: %s", name)
		}
	}
	// Sequences must also survive: inserting a ledger record must not collide.
	if _, err := restored.Exec(ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES(0,true)`); err != nil {
		t.Fatal(err)
	}
}
