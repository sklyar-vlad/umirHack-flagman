package database

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
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

func seedMonitoring(t *testing.T, f *fixture) {
	t.Helper()
	_, err := f.pool.Exec(f.ctx, `
 INSERT INTO users(id,name,email,password_hash,role) VALUES('10000000-0000-4000-8000-000000000001','Test','views@example.test','test','admin');
 INSERT INTO product_categories(id,name) VALUES('20000000-0000-4000-8000-000000000001','Test');
 INSERT INTO products(id,category_id,name,unit,display_unit,target_stock) VALUES
 ('30000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001','Flour','g','g',20),
 ('30000000-0000-4000-8000-000000000002','20000000-0000-4000-8000-000000000001','Egg','pcs','pcs',10);
 INSERT INTO supplies(id,number,received_at,created_by) VALUES('50000000-0000-4000-8000-000000000001','test',clock_timestamp(),'10000000-0000-4000-8000-000000000001');
 INSERT INTO restaurant_settings(id,restaurant_name,expiring_threshold_hours) VALUES(1,'Test',24);
 INSERT INTO batches(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at) VALUES
 ('60000000-0000-4000-8000-000000000001','50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001',1,9,9,'explicit',clock_timestamp()+interval '48 hours'),
 ('60000000-0000-4000-8000-000000000002','50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001',2,3,3,'explicit',clock_timestamp()-interval '1 hour'),
 ('60000000-0000-4000-8000-000000000003','50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001',3,2,2,'explicit',clock_timestamp()+interval '1 hour'),
 ('60000000-0000-4000-8000-000000000004','50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002',4,5,0,'explicit',clock_timestamp()-interval '1 hour');
 INSERT INTO dishes(id,name,is_active) VALUES
 ('40000000-0000-4000-8000-000000000001','Short',true),
 ('40000000-0000-4000-8000-000000000002','Missing',true),
 ('40000000-0000-4000-8000-000000000003','Inactive',false);
 INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES
 ('70000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001',1,'10000000-0000-4000-8000-000000000001');
 INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES
 ('70000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001',12);
 INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,note,occurred_at) VALUES
 ('60000000-0000-4000-8000-000000000002','manual_writeoff',-1,3,'Historical flour','g','Test',clock_timestamp()),
 ('60000000-0000-4000-8000-000000000002','expiration',-2,1,'Historical flour','g','Test',clock_timestamp()),
 ('60000000-0000-4000-8000-000000000004','manual_writeoff',-5,0,'Historical eggs','pcs','Test',clock_timestamp()),
 ('60000000-0000-4000-8000-000000000001','receipt',9,9,'Flour','g','Test',clock_timestamp());`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMonitoringViewsCurrentStockRecipesAndHistoricalUnits(t *testing.T) {
	f := newFixture(t)
	seedMonitoring(t, f)
	var valid, expired string
	if err := f.pool.QueryRow(f.ctx, `SELECT valid_quantity::text,expired_quantity::text FROM monitoring_stock WHERE product_name='Flour'`).
		Scan(&valid, &expired); err != nil ||
		valid != "11.000" ||
		expired != "3.000" {
		t.Fatal(valid, expired, err)
	}
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM monitoring_problem_batches`).Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM monitoring_stop_list`).Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	var g, pcs string
	if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE(sum(quantity) FILTER(WHERE unit='g'),0)::text,COALESCE(sum(quantity) FILTER(WHERE unit='pcs'),0)::text FROM monitoring_losses`).
		Scan(&g, &pcs); err != nil ||
		g != "3.000" ||
		pcs != "5.000" {
		t.Fatal(g, pcs, err)
	}
	_, err := f.pool.Exec(f.ctx, `UPDATE recipe_items SET quantity_per_portion=11`)
	if err != nil {
		t.Fatal(err)
	}
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM monitoring_stop_list`).Scan(&n)
	if n != 1 {
		t.Fatal("not recalculated", n)
	}
	_, err = f.pool.Exec(
		f.ctx,
		`UPDATE batches SET expires_at=clock_timestamp()-interval '1 second' WHERE line_number=3`,
	)
	if err != nil {
		t.Fatal(err)
	}
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM monitoring_stop_list`).Scan(&n)
	if n != 2 {
		t.Fatal("expiry not recalculated", n)
	}
}

func TestMonitoringRoleProvisioningAndForbiddenWrites(t *testing.T) {
	if os.Getenv("FLAGMAN_TEST_MONITORING") != "1" {
		t.Skip("set FLAGMAN_TEST_MONITORING=1 for psql container checks")
	}
	f := newFixture(t)
	seedMonitoring(t, f)
	cfg := f.pool.Config()
	role := "monitoring_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	var schema string
	if err := f.pool.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../infrastructure/monitoring/grafana-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(sql), "flagman_grafana", role)
	script = strings.ReplaceAll(script, "SCHEMA public", "SCHEMA "+pgx.Identifier{schema}.Sanitize())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "setup.sql"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"run",
		"--rm",
		"--network=host",
		"--read-only",
		"--cap-drop=ALL",
		"-v",
		dir + ":/test:ro",
		"-e",
		"PGHOST",
		"-e",
		"PGPORT",
		"-e",
		"PGDATABASE",
		"-e",
		"PGUSER",
		"-e",
		"PGPASSWORD",
		"-e",
		"PGOPTIONS",
		"-e",
		"FLAGMAN_GRAFANA_PASSWORD",
		"postgres:18-alpine",
		"psql",
		"-X",
		"-v",
		"ON_ERROR_STOP=1",
		"-f",
		"/test/setup.sql",
	}
	cmd := exec.CommandContext(f.ctx, "docker", args...)
	cmd.Env = append(
		os.Environ(),
		"PGHOST="+cfg.ConnConfig.Host,
		"PGPORT="+strconv.Itoa(int(cfg.ConnConfig.Port)),
		"PGDATABASE="+cfg.ConnConfig.Database,
		"PGUSER="+cfg.ConnConfig.User,
		"PGPASSWORD="+cfg.ConnConfig.Password,
		"PGOPTIONS=-c search_path="+schema,
		"FLAGMAN_GRAFANA_PASSWORD="+password,
	)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = f.pool.Exec(ctx, "DROP OWNED BY "+pgx.Identifier{role}.Sanitize())
		_, _ = f.pool.Exec(ctx, "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	})
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("role setup failed: %v %s", err, out)
	}
	login := cfg.Copy()
	login.ConnConfig.User = role
	login.ConnConfig.Password = password
	pool, err := pgxpool.NewWithConfig(f.ctx, login)
	if err != nil {
		t.Fatal("reader connection failed")
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(f.ctx, `SELECT count(*) FROM monitoring_stock`).Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	for _, query := range []string{`SELECT * FROM users`, `SELECT * FROM sessions`, `SELECT * FROM audit_log`, `SELECT * FROM products`, `UPDATE monitoring_stock SET product_name='Changed'`, `INSERT INTO products(name) VALUES('Bad')`, `CREATE TABLE forbidden(id int)`} {
		if _, err := pool.Exec(f.ctx, query); err == nil {
			t.Fatal("reader allowed forbidden operation", query)
		}
	}
	if _, err := pool.Exec(f.ctx, `SET default_transaction_read_only=off`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(f.ctx, `UPDATE products SET name='Bad'`); err == nil {
		t.Fatal("reader has underlying write privileges")
	}
	verifyGrafana(t, f, role, password, schema)
}

func verifyGrafana(t *testing.T, f *fixture, role, password, schema string) {
	t.Helper()
	if _, err := f.pool.Exec(
		f.ctx,
		"ALTER ROLE "+pgx.Identifier{role}.Sanitize()+" SET search_path="+pgx.Identifier{schema}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}
	cfg := f.pool.Config()
	dir := t.TempDir()
	sources := filepath.Join(dir, "provisioning", "datasources")
	providers := filepath.Join(dir, "provisioning", "dashboards")
	for _, path := range []string{sources, providers} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("../../infrastructure/monitoring/grafana/provisioning/datasources/flagman.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ReplaceAll(
		string(data),
		"db:5432",
		net.JoinHostPort(cfg.ConnConfig.Host, strconv.Itoa(int(cfg.ConnConfig.Port))),
	)
	source = strings.ReplaceAll(source, "flagman_grafana", role)
	if err := os.WriteFile(filepath.Join(sources, "flagman.yaml"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile("../../infrastructure/monitoring/grafana/provisioning/dashboards/flagman.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providers, "flagman.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	adminPassword := uuid.NewString()
	for name, value := range map[string]string{"grafana_admin_password": adminPassword, "grafana_pg_password": password} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listen.Addr().(*net.TCPAddr).Port
	listen.Close()
	root, err := filepath.Abs("../../infrastructure/monitoring/grafana")
	if err != nil {
		t.Fatal(err)
	}
	name := "flagman-grafana-test-" + uuid.NewString()
	cmd := exec.Command(
		"docker",
		"run",
		"-d",
		"--name",
		name,
		"--network=host",
		"--read-only",
		"--cap-drop=ALL",
		"--tmpfs",
		"/var/lib/grafana:uid=472,gid=472",
		"--tmpfs",
		"/var/log/grafana:uid=472,gid=472",
		"-v",
		dir+"/provisioning:/etc/grafana/provisioning:ro",
		"-v",
		root+"/dashboards:/etc/grafana/dashboards:ro",
		"-v",
		root+"/entrypoint.sh:/etc/grafana/entrypoint.sh:ro",
		"-v",
		dir+"/grafana_admin_password:/run/secrets/grafana_admin_password:ro",
		"-v",
		dir+"/grafana_pg_password:/run/secrets/grafana_pg_password:ro",
		"-e",
		"POSTGRES_DB="+cfg.ConnConfig.Database,
		"-e",
		"GF_SERVER_HTTP_ADDR=127.0.0.1",
		"-e",
		"GF_SERVER_HTTP_PORT="+strconv.Itoa(port),
		"-e",
		"GF_SECURITY_ADMIN_PASSWORD__FILE=/run/secrets/grafana_admin_password",
		"-e",
		"GF_AUTH_ANONYMOUS_ENABLED=false",
		"-e",
		"GF_USERS_ALLOW_SIGN_UP=false",
		"-e",
		"GF_PLUGINS_PREINSTALL_DISABLED=true",
		"-e",
		"GF_LOG_LEVEL=warn",
		"-e",
		"GF_ANALYTICS_REPORTING_ENABLED=false",
		"-e",
		"GF_ANALYTICS_CHECK_FOR_UPDATES=false",
		"-e",
		"GF_ANALYTICS_CHECK_FOR_PLUGIN_UPDATES=false",
		"--entrypoint",
		"/bin/sh",
		"grafana/grafana:12.1.1",
		"/etc/grafana/entrypoint.sh",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Grafana test startup: %v %s", err, out)
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
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: 2 * time.Second}
	until := time.Now().Add(30 * time.Second)
	for {
		resp, err := client.Get(base + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(until) {
			t.Fatal("Grafana not ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	request := func(method, path, body string, auth bool) (int, []byte) {
		t.Helper()
		r, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.SetBasicAuth("admin", adminPassword)
		}
		resp, err := client.Do(r)
		if err != nil {
			if method == http.MethodGet && path == "/api/dashboards/uid/flagman" {
				return 0, nil
			}
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	code, _ := request("GET", "/api/search", "", false)
	if code != 401 {
		t.Fatal("anonymous dashboard access", code)
	}
	for {
		code, data = request("GET", "/api/dashboards/uid/flagman", "", true)
		if code == 200 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("dashboard provisioning timeout", code)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if code != 200 || !strings.Contains(string(data), "Stop list") {
		t.Fatal("dashboard not provisioned", code, string(data))
	}
	code, data = request("GET", "/api/datasources/uid/flagman-warehouse", "", true)
	if code != 200 || strings.Contains(string(data), password) {
		t.Fatal("source missing or secret exposed", code)
	}
	query := `{"from":"0","to":"9999999999999","queries":[{"refId":"A","datasource":{"uid":"flagman-warehouse","type":"postgres"},"format":"table","rawQuery":true,"rawSql":"SELECT product_name,unit,valid_quantity,expired_quantity FROM monitoring_stock ORDER BY product_name"}]}`
	code, data = request("POST", "/api/ds/query", query, true)
	var response struct {
		Results map[string]struct {
			Error  string
			Frames []json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if code != 200 || response.Results["A"].Error != "" || len(response.Results["A"].Frames) != 1 ||
		!strings.Contains(string(data), "Flour") {
		t.Fatal("SQL dashboard query failed", code, string(data))
	}
}
