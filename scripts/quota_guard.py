#!/usr/bin/env python3
"""Pause the local workflow with a five-percent subscription quota reserve."""
from __future__ import annotations

import json
import math
import os
from pathlib import Path
import select
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
STATE = ROOT / ".ralph" / "quota.json"
PAUSED = ROOT / ".ralph" / "paused.json"
RESERVE = 5


class QuotaUnavailable(RuntimeError):
    pass


class QuotaClient:
    """Read usage through the supported app-server account API, without a turn."""
    def __init__(self):
        self.process = subprocess.Popen(
            ["codex", "app-server", "--stdio"], cwd=ROOT,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, start_new_session=True,
        )
        self.buffer = b""
        self.request_id = 0
        try:
            self.request("initialize", {"clientInfo": {
                "name": "local_quota_guard", "title": "Local quota guard", "version": "1.0.0"
            }})
            self.send({"method": "initialized", "params": {}})
        except Exception:
            self.close()
            raise

    def send(self, message):
        try:
            self.process.stdin.write((json.dumps(message) + "\n").encode())
            self.process.stdin.flush()
        except (BrokenPipeError, OSError) as exc:
            raise QuotaUnavailable("Quota connection closed") from exc

    def request(self, method, params=None):
        self.request_id += 1
        message = {"id": self.request_id, "method": method}
        if params is not None:
            message["params"] = params
        self.send(message)
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            if b"\n" not in self.buffer:
                readable, _, _ = select.select([self.process.stdout], [], [], max(0, deadline-time.monotonic()))
                if not readable:
                    break
                chunk = os.read(self.process.stdout.fileno(), 65536)
                if not chunk:
                    break
                self.buffer += chunk
                continue
            line, self.buffer = self.buffer.split(b"\n", 1)
            try:
                response = json.loads(line)
            except ValueError:
                continue
            if response.get("id") != self.request_id:
                continue
            if "error" in response:
                # Never print account responses, authentication or raw server errors.
                raise QuotaUnavailable("Quota API rejected the request")
            return response.get("result", {})
        raise QuotaUnavailable("Quota API did not respond within 25 seconds")

    def read(self):
        for attempt in range(3):
            try:
                return summarize(self.request("account/rateLimits/read"))
            except QuotaUnavailable:
                if attempt == 2:
                    raise
                time.sleep(1)

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        for stream in (self.process.stdin, self.process.stdout):
            if stream:
                stream.close()


def summarize(result):
    buckets = result.get("rateLimitsByLimitId") or {}
    bucket = buckets.get("codex") or result.get("rateLimits") or {}
    windows = []
    for name in ("primary", "secondary"):
        value = bucket.get(name)
        if not isinstance(value, dict):
            continue
        used, minutes, reset = value.get("usedPercent"), value.get("windowDurationMins"), value.get("resetsAt")
        if (isinstance(used, bool) or not isinstance(used, (int, float))
                or not math.isfinite(used) or not 0 <= used <= 100
                or not isinstance(minutes, int) or minutes <= 0
                or not isinstance(reset, int) or reset <= 0):
            raise QuotaUnavailable("Quota API returned an invalid window")
        windows.append({"name": name, "used_percent": used,
                        "remaining_percent": 100-used, "window_minutes": minutes, "resets_at": reset})
    if not any(window["window_minutes"] == 300 for window in windows):
        raise QuotaUnavailable("The five-hour quota window is unavailable")
    return {"checked_at": int(time.time()), "reserve_percent": RESERVE, "windows": windows}


def pause_reason(quota):
    for window in quota["windows"]:
        if window["remaining_percent"] <= RESERVE:
            return {"reason": "quota_reserve", "window": window}
    return None


def write_json(filename, value):
    filename.parent.mkdir(parents=True, exist_ok=True)
    temporary = filename.with_suffix(".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    os.replace(temporary, filename)


def descendants(parent):
    entries = {}
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            stat = (entry / "stat").read_text().rsplit(")", 1)[1].split()
            entries[int(entry.name)] = (int(stat[1]), stat[19])
        except (OSError, ValueError, IndexError):
            pass
    children, frontier = {}, {parent}
    while frontier:
        next_frontier = {pid for pid, (ppid, _) in entries.items() if ppid in frontier and pid not in children}
        for pid in next_frontier:
            children[pid] = entries[pid][1]
        frontier = next_frontier
    return children


def interrupt_executors(parent):
    # Interrupt only Codex executors owned by this workflow; other sessions continue.
    executors = []
    for pid, start_time in descendants(parent).items():
        try:
            entry = Path(f"/proc/{pid}")
            args = (entry / "cmdline").read_bytes().split(b"\0")
            current_start = (entry / "stat").read_text().rsplit(")", 1)[1].split()[19]
            if current_start == start_time and Path(os.fsdecode(args[0])).name == "codex" and b"exec" in args:
                os.kill(pid, signal.SIGINT)
                executors.append(pid)
        except (OSError, ValueError, IndexError):
            pass
    # Give Codex time to finish interruption and persist its session.
    deadline = time.monotonic() + 20
    while executors and time.monotonic() < deadline:
        executors = [pid for pid in executors if Path(f"/proc/{pid}").exists()]
        if executors:
            time.sleep(0.2)


def stop_workflow(process):
    interrupt_executors(process.pid)
    if process.poll() is None:
        process.terminate()  # Ralph's EXIT handler cleans its isolated executor groups.
        try:
            process.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()


def checkpoint(reason):
    value = {"status": "paused", "paused_at": int(time.time()), **reason}
    for name, args in (("head", ["git", "rev-parse", "HEAD"]),
                       ("working_tree", ["git", "status", "--porcelain"]),
                       ("tasks", ["bd", "--sandbox", "list", "--status", "in_progress", "--json"])):
        try:
            result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=15)
            if result.returncode == 0:
                value[name] = json.loads(result.stdout) if name == "tasks" else result.stdout.strip()
        except (OSError, ValueError, subprocess.TimeoutExpired):
            pass
    write_json(PAUSED, value)
    print("Workflow paused; files and unfinished tasks are preserved. Use the resume launcher.", flush=True)


def main():
    args = sys.argv[1:]
    read_only = args == ["--status"]
    prepare = args[:1] == ["--prepare"]
    if prepare:
        args = args[1:]
    if args[:1] == ["--"]:
        args = args[1:]
    if not read_only and not args:
        raise SystemExit("Usage: quota_guard.py --status | -- COMMAND [ARGS...]")
    client, process = None, None
    requested = []
    def request_stop(signum, _frame):
        requested.append(signum)
    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)
    try:
        client = QuotaClient()
        quota = client.read()
        if read_only:
            print(json.dumps(quota, ensure_ascii=False, indent=2))
            return 0
        write_json(STATE, quota)
        reason = pause_reason(quota)
        if reason:
            checkpoint(reason)
            return 0
        PAUSED.unlink(missing_ok=True)
        if prepare:
            result = subprocess.run([str(ROOT / "scripts/yougile-queue.sh"), "prepare"],
                                    cwd=ROOT, capture_output=True, text=True, check=True, timeout=300)
            print(result.stdout.strip(), flush=True)
            if json.loads(result.stdout)["status"] == "drained":
                return 0
        process = subprocess.Popen(args, cwd=ROOT, start_new_session=True)
        print("Quota guard enabled: pause at <=5% remaining; check every 15 seconds.", flush=True)
        while process.poll() is None:
            if requested:
                stop_workflow(process)
                checkpoint({"reason": "manual_pause"})
                return 0
            quota = client.read()
            write_json(STATE, quota)
            reason = pause_reason(quota)
            if reason:
                # Publish the flag before interruption so retries cannot start another turn.
                write_json(PAUSED, {"status": "pausing", **reason})
                stop_workflow(process)
                checkpoint(reason)
                return 0
            for _ in range(30):
                if process.poll() is not None or requested:
                    break
                time.sleep(0.5)
        return process.returncode
    except (QuotaUnavailable, OSError) as exc:
        if read_only:
            print("Quota status is temporarily unavailable; the running workflow is unchanged.", file=sys.stderr)
            return 1
        if process is not None and process.poll() is None:
            stop_workflow(process)
        checkpoint({"reason": "quota_unavailable", "detail": str(exc) if isinstance(exc, QuotaUnavailable) else "Local quota reader failed"})
        return 0
    finally:
        if client:
            client.close()


if __name__ == "__main__":
    sys.exit(main())
