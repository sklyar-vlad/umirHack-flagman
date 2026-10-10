"""Check the actual executor sandbox before spending an iteration on a story."""
import json
import os
import socket
import subprocess
import sys
import time
from pathlib import Path
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parent.parent


def database_address():
    address = urlparse(os.environ["FLAGMAN_TEST_DATABASE_URL"])
    if address.hostname not in ("localhost", "127.0.0.1", "::1") or "test" not in address.path:
        raise RuntimeError("A dedicated local test database is required")
    return address.hostname, address.port or 5432


def check_database():
    with socket.create_connection(database_address(), timeout=5):
        pass


def main():
    if sys.argv[1:] == ["--inside-sandbox"]:
        check_database()
        for variable in ("GOCACHE", "UV_CACHE_DIR", "UV_TOOL_DIR", "GOPATH"):
            probe = Path(os.environ[variable]) / f".workflow-write-probe-{os.getpid()}"
            probe.write_text("ok")
            probe.unlink()
        subprocess.run([str(ROOT / "scripts/yougile-queue.sh"), "check"], cwd=ROOT,
                       check=True, timeout=60)
        print("Preflight: PostgreSQL, YouGile and writable caches verified inside the Codex sandbox.", flush=True)
        return
    try:
        check_database()
    except OSError:
        if database_address() != ("127.0.0.1", 55438):
            raise RuntimeError("Dedicated test PostgreSQL is unavailable") from None
        container = "flagman-s08-test"
        info = json.loads(subprocess.check_output(["docker", "inspect", container], text=True))[0]
        ports = info["HostConfig"]["PortBindings"].get("5432/tcp", [])
        if not info["Config"]["Image"].startswith("postgres:") or ports != [{"HostIp": "127.0.0.1", "HostPort": "55438"}]:
            raise RuntimeError("Unexpected test container configuration")
        subprocess.run(["docker", "start", container], check=True, stdout=subprocess.DEVNULL, timeout=30)
        for attempt in range(20):
            try:
                check_database()
                break
            except OSError:
                if attempt == 19:
                    raise RuntimeError("Dedicated test PostgreSQL did not start") from None
                time.sleep(1)
    writable = [os.environ[name] for name in ("UV_CACHE_DIR", "UV_TOOL_DIR", "GOPATH")]
    subprocess.run(["codex", "sandbox", "-c", "sandbox_mode=workspace-write",
                    "-c", "sandbox_workspace_write.network_access=true",
                    "-c", "sandbox_workspace_write.writable_roots=" + json.dumps(writable),
                    "--", "python3", str(Path(__file__).resolve()), "--inside-sandbox"],
                   cwd=ROOT, check=True, timeout=90)


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.SubprocessError, ValueError, KeyError) as error:
        print("Workflow preflight failed: " + type(error).__name__ + ". See the check above; no coding iteration started.", file=sys.stderr)
        from quota_guard import checkpoint
        checkpoint({"reason": "preflight_failed", "detail": "Test database, YouGile or executor permissions unavailable"})
        sys.exit(1)
