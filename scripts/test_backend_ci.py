"""Safety and failure contracts for the reusable backend check entry points."""
import os
import importlib.util
import json
from pathlib import Path
import subprocess
import signal
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "results", ROOT / "scripts/check-backend-test-results.py"
)
results = importlib.util.module_from_spec(spec)
spec.loader.exec_module(results)


class BackendChecksTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.bin = Path(self.temp.name)
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}")

    def stub(self, name, body):
        path = self.bin / name
        path.write_text("#!/usr/bin/env bash\nset -eu\n" + body + "\n")
        path.chmod(0o700)

    def run_check(self, action):
        return subprocess.run(
            ["bash", str(ROOT / "scripts/backend-checks.sh"), action],
            env=self.env, capture_output=True, text=True, check=False,
        )

    def test_format_reports_files_without_rewriting_them(self):
        self.stub("gofmt", 'test "$1" = -l; echo internal/unformatted.go')
        result = self.run_check("format")
        self.assertEqual(result.returncode, 1)
        self.assertIn("internal/unformatted.go", result.stderr)

    def test_unit_checks_remove_every_inherited_integration_destination(self):
        names = ["FLAGMAN", "PRODUCT", "SUPPLY", "MENU", "BATCH", "INVENTORY"]
        for name in names:
            self.env[f"{name}_TEST_DATABASE_URL"] = "must-not-connect"
        self.env["FLAGMAN_TEST_HTTPS"] = "1"
        self.stub("go", '''
test "$*" = 'test -count=1 ./...'
for name in FLAGMAN PRODUCT SUPPLY MENU BATCH INVENTORY; do
    key="${name}_TEST_DATABASE_URL"
    test -z "${!key:-}"
done
test -z "${FLAGMAN_TEST_HTTPS:-}"
''')
        self.assertEqual(self.run_check("unit").returncode, 0)

    def test_lint_failure_stops_before_tests_or_build(self):
        marker = self.bin / "unexpected-go"
        self.env["FAILURE_MARKER"] = str(marker)
        self.stub("gofmt", "exit 0")
        self.stub("golangci-lint", "exit 17")
        self.stub("go", 'touch "$FAILURE_MARKER"')
        self.assertEqual(self.run_check("all").returncode, 17)
        self.assertFalse(marker.exists())

    def test_invalid_action_fails(self):
        self.assertEqual(self.run_check("unknown").returncode, 2)

    def test_integration_rejects_remote_docker_before_creating_resources(self):
        self.env["DOCKER_HOST"] = "ssh://must-not-contact"
        marker = self.bin / "unexpected-docker"
        self.env["FAILURE_MARKER"] = str(marker)
        self.stub("docker", 'touch "$FAILURE_MARKER"')
        self.stub("goose", "exit 0")
        process = subprocess.run(
            ["bash", str(ROOT / "scripts/backend-ci.sh"), "integration"],
            env=self.env, capture_output=True, text=True, check=False,
        )
        self.assertNotEqual(process.returncode, 0)
        self.assertIn("local Unix Docker socket", process.stderr)
        self.assertFalse(marker.exists())

    def test_failed_migration_cleans_owned_stack_and_keeps_secrets_out_of_output(self):
        self.env["DOCKER_HOST"] = "unix:///var/run/docker.sock"
        self.env["HARNESS_LOG"] = str(self.bin / "calls")
        self.env["HARNESS_TEMP"] = str(self.bin / "temp-path")
        self.stub("docker", '''
printf '%s\\n' "$*" >> "$HARNESS_LOG"
printf '%s' "$FLAGMAN_CI_DIR" > "$HARNESS_TEMP"
case "$*" in
    *'port db 5432') echo '127.0.0.1:54321' ;;
esac
''')
        self.stub("goose", "echo must-not-print-migration-credentials >&2; exit 23")
        process = subprocess.run(
            ["bash", str(ROOT / "scripts/backend-ci.sh"), "integration"],
            env=self.env, capture_output=True, text=True, check=False,
        )
        self.assertNotEqual(process.returncode, 0)
        self.assertNotIn("must-not-print", process.stdout + process.stderr)
        self.assertIn("--profile smoke down --volumes --remove-orphans", (self.bin / "calls").read_text())
        self.assertFalse(Path((self.bin / "temp-path").read_text()).exists())

    def test_termination_removes_only_owned_resources(self):
        self.env["DOCKER_HOST"] = "unix:///var/run/docker.sock"
        self.env["HARNESS_LOG"] = str(self.bin / "calls")
        self.env["HARNESS_TEMP"] = str(self.bin / "temp-path")
        ready = self.bin / "ready"
        self.env["HARNESS_READY"] = str(ready)
        self.stub("docker", '''
printf '%s\\n' "$*" >> "$HARNESS_LOG"
printf '%s' "$FLAGMAN_CI_DIR" > "$HARNESS_TEMP"
case "$*" in
    *'port db 5432') echo '127.0.0.1:54321' ;;
esac
''')
        self.stub("goose", 'touch "$HARNESS_READY"; sleep 30')
        process = subprocess.Popen(
            ["bash", str(ROOT / "scripts/backend-ci.sh"), "integration"],
            env=self.env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        try:
            deadline = time.monotonic() + 5
            while not ready.exists() and time.monotonic() < deadline:
                time.sleep(0.02)
            self.assertTrue(ready.exists())
            os.killpg(process.pid, signal.SIGTERM)
            self.assertEqual(process.wait(timeout=5), 143)
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)
        calls = (self.bin / "calls").read_text()
        self.assertIn("--profile smoke down --volumes --remove-orphans", calls)
        self.assertNotIn("flagman-postgres", calls)
        self.assertNotIn("infrastructure/.env", calls)
        self.assertFalse(Path((self.bin / "temp-path").read_text()).exists())


class IntegrationResultsTest(unittest.TestCase):
    def successful_events(self):
        return [json.dumps({"Action": "pass", "Package": results.PREFIX + package, "Test": test})
                for package, tests in results.REQUIRED.items() for test in tests]

    def test_required_tests_and_explicit_optional_skip_pass(self):
        events = self.successful_events()
        events.append(json.dumps({"Action": "skip", "Test": "TestBackupRestore"}))
        passed, skipped, problems = results.review(events)
        self.assertTrue(passed)
        self.assertEqual(skipped, 1)
        self.assertEqual(problems, [])

    def test_missing_database_tests_cannot_silently_pass(self):
        self.assertTrue(results.review([])[2])
        events = self.successful_events()[1:]
        self.assertTrue(results.review(events)[2])

    def test_failed_packages_and_unexpected_skips_fail(self):
        for event in [{"Action": "fail", "Package": "compile-error"},
                      {"Action": "skip", "Test": "TestNewPostgresTransaction"}]:
            events = self.successful_events() + [json.dumps(event)]
            self.assertTrue(results.review(events)[2])


if __name__ == "__main__":
    unittest.main()
