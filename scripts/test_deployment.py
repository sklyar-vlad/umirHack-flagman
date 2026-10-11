"""Run with: python3 -m unittest discover -s scripts -p test_deployment.py."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="flagman-deployment-test-")
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name)
        self.env_file = self.path / "deploy.env"
        text = (ROOT / "infrastructure/.env.vps.example").read_text()
        for name in (
            "POSTGRES_PASS", "METRICS_TOKEN", "GRAFANA_ADMIN_PASSWORD",
            "GRAFANA_PG_PASSWORD", "DEMO_ADMIN_PASSWORD", "DEMO_MANAGER_PASSWORD",
            "DEMO_STOREKEEPER_PASSWORD", "DEMO_ORDER_OPERATOR_PASSWORD",
        ):
            text = text.replace(name + "=\n", name + "=" + "1234abcd" * 8 + "\n")
        self.env_file.write_text(text)
        self.env_file.chmod(0o600)

    def compose(self):
        result = subprocess.run(
            ["docker", "compose", "--env-file", str(self.env_file),
             "-f", "infrastructure/compose.vps.yaml", "--profile", "ops",
             "config", "--format", "json"],
            cwd=ROOT, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, "Compose configuration rejected")
        return json.loads(result.stdout)

    def test_database_url_interpolation_and_startup_order(self):
        services = self.compose()["services"]
        dsn = services["backend"]["environment"]["POSTGRES_URL"]
        self.assertIn("@db:5432/flagman?sslmode=disable", dsn)
        self.assertNotIn("${", dsn)
        for name in ("migrate", "seed-users"):
            self.assertEqual(services[name]["environment"]["GOOSE_DBSTRING"], dsn)
        self.assertEqual(
            services["backend"]["depends_on"]["migrate"]["condition"],
            "service_completed_successfully",
        )
        self.assertEqual(
            services["frontend"]["depends_on"]["backend"]["condition"],
            "service_healthy",
        )
        self.assertEqual(
            services["grafana"]["depends_on"]["monitoring-init"]["condition"],
            "service_completed_successfully",
        )

    def test_tls_persistence_and_private_ports(self):
        config = self.compose()
        services = config["services"]
        self.assertFalse(config.get("secrets"))
        for name in ("db", "backend", "prometheus"):
            self.assertFalse(services[name].get("ports"))
        for name in ("grafana",):
            self.assertTrue(all(p["host_ip"] == "127.0.0.1" for p in services[name]["ports"]))
        self.assertEqual(
            services["frontend"]["labels"]["traefik.http.routers.frontend.tls.certresolver"],
            "letsencrypt",
        )
        self.assertTrue(any(v["target"] == "/letsencrypt" and v["type"] == "volume"
                            for v in services["traefik"]["volumes"]))
        self.assertIn("--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web",
                      services["traefik"]["command"])
        self.assertNotEqual(services["traefik"].get("labels", {}).get("traefik.enable"), "true")
        self.assertIn("--providers.file.filename=/etc/traefik/dashboard.yaml",
                      services["traefik"]["command"])

    def test_invalid_metrics_or_domain_are_rejected_without_secret_output(self):
        for token, domain, accepted in (
            ("secret-too-short", "warehouse.example.com", False),
            ("a" * 32, "warehouse.example.com", True),
            ("a" * 32, "https://warehouse.example.com", False),
            ("a" * 32, "warehouse.example.com:8444", False),
        ):
            with self.subTest(token_length=len(token), domain=domain):
                env = os.environ.copy()
                env.update(METRICS_TOKEN=token, FLAGMAN_DOMAIN=domain)
                result = subprocess.run(["sh", "infrastructure/validate-vps-env.sh"],
                                        cwd=ROOT, env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode == 0, accepted)
                self.assertNotIn(token, result.stdout + result.stderr)

    def fake_tools(self):
        tools = self.path / "bin"
        tools.mkdir()
        self.log = self.path / "calls.jsonl"
        self.log.write_text("")
        for name in ("docker", "goose", "go"):
            tool = tools / name
            tool.write_text(
                "#!/usr/bin/env python3\nimport json,os,sys\n"
                "with open(os.environ['CALL_LOG'], 'a') as f:\n"
                " f.write(json.dumps([sys.argv[0].split('/')[-1]] + sys.argv[1:]) + '\\n')\n"
            )
            tool.chmod(0o755)
        env = os.environ.copy()
        env.update(PATH=str(tools) + os.pathsep + env["PATH"],
                   FLAGMAN_ENV_FILE=str(self.env_file), FLAGMAN_MODE="auto",
                   CALL_LOG=str(self.log))
        return env

    def run_task(self, action, env):
        self.log.write_text("")
        result = subprocess.run(["sh", "infrastructure/manage.sh", action], cwd=ROOT,
                                env=env, capture_output=True)
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        return result, calls

    def test_vps_tasks_use_containers_and_selected_env(self):
        env = self.fake_tools()
        for action in ("up", "migrate", "migrate-status", "seed", "seed-users", "down"):
            with self.subTest(action=action):
                result, calls = self.run_task(action, env)
                self.assertEqual(result.returncode, 0)
                self.assertTrue(calls)
                for call in calls:
                    self.assertEqual(call[0], "docker")
                    self.assertIn(str(self.env_file), call)
                    self.assertTrue(any(arg.endswith("compose.vps.yaml") for arg in call))
                if action in ("seed", "seed-users"):
                    self.assertTrue(any("run" in call and "seed-users" in call for call in calls))
                if action == "seed":
                    self.assertEqual(sum("exec" in call for call in calls), 2)
                if action == "up":
                    validation = next(i for i, call in enumerate(calls) if "validate-config" in call)
                    stop = next(i for i, call in enumerate(calls) if "stop" in call)
                    self.assertLess(validation, stop)

    def test_local_migrations_keep_host_goose(self):
        env = self.fake_tools()
        self.env_file.write_text("POSTGRES_DB=local\n")
        result, calls = self.run_task("migrate", env)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(calls[0][0], "goose")
        self.assertIn(str(self.env_file), calls[0])

    def test_vps_rollback_and_invalid_selector_do_not_run_commands(self):
        env = self.fake_tools()
        for action in ("rollback", "rollback-one", "migrate-one"):
            result, calls = self.run_task(action, env)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(calls)
        marker = self.path / "must-not-exist"
        self.env_file.write_text(f"DEPLOY_MODE=$(touch {marker})\n")
        result, calls = self.run_task("up", env)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(calls)
        self.assertFalse(marker.exists())

    @unittest.skipUnless(shutil.which("task"), "Task is not installed")
    def test_taskfile_passes_custom_env_and_mode(self):
        env = self.fake_tools()
        env.pop("FLAGMAN_MODE")
        env.pop("FLAGMAN_ENV_FILE")
        # A local-looking file must still use VPS when explicitly selected.
        self.env_file.write_text("POSTGRES_DB=local\n")
        result = subprocess.run(
            ["task", "migrate", "MODE=vps", "ENV_FILE=" + str(self.env_file)],
            cwd=ROOT, env=env, capture_output=True,
        )
        self.assertEqual(result.returncode, 0)
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertTrue(all(call[0] == "docker" for call in calls))
        self.assertTrue(all(str(self.env_file) in call for call in calls))


if __name__ == "__main__":
    unittest.main()
