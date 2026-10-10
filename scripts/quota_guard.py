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
PAUSE_METADATA = "workflow_pause"


class QuotaUnavailable(RuntimeError):
    pass


class TaskStateUnavailable(RuntimeError):
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


def beads(*args):
    try:
        result = subprocess.run(["bd", "--sandbox", *args], cwd=ROOT,
                                capture_output=True, text=True, timeout=10, check=True)
        return result.stdout
    except (OSError, subprocess.SubprocessError) as exc:
        raise TaskStateUnavailable("Could not update local task state") from exc


def task_metadata(task):
    metadata = task.get("metadata") or {}
    if isinstance(metadata, str):
        metadata = json.loads(metadata)
    return dict(metadata)


def owned_pause(task):
    marker = task_metadata(task).get(PAUSE_METADATA)
    return (isinstance(marker, dict) and marker.get("owner") == "local_workflow"
            and marker.get("previous_status") == "in_progress")


def block_active_tasks(reason):
    """Block only work owned by this queue; leave waiting and closed tasks intact."""
    blocked = []
    for task in json.loads(beads("list", "--all", "--limit", "0", "--json")):
        if not task.get("external_ref", "").startswith("yougile:"):
            continue
        if task["status"] == "blocked" and owned_pause(task):
            blocked.append(task)
            continue
        if task["status"] != "in_progress":
            continue
        metadata = task_metadata(task)
        marker = {"owner": "local_workflow", "previous_status": "in_progress",
                  "reason": reason["reason"], "paused_at": reason["paused_at"]}
        if reason.get("window"):
            marker["window"] = reason["window"]
        metadata[PAUSE_METADATA] = marker
        note = "Пауза выполнения: " + reason["reason"] + "."
        if reason.get("window"):
            note += " Остаток квоты: " + str(reason["window"]["remaining_percent"]) + "%."
        beads("update", task["id"], "--if-status", "in_progress", "--status", "blocked",
              "--metadata", json.dumps(metadata), "--append-notes", note)
        blocked.append({**task, "status": "blocked", "metadata": metadata})
    return blocked


def resume_paused_tasks():
    """Restore the guard's own pauses after a successful fresh quota check."""
    for task in json.loads(beads("list", "--all", "--limit", "0", "--json")):
        if (task.get("external_ref", "").startswith("yougile:")
                and task["status"] == "blocked" and owned_pause(task)):
            beads("update", task["id"], "--if-status", "blocked", "--status", "in_progress",
                  "--unset-metadata", PAUSE_METADATA, "--append-notes",
                  "Работа возобновлена после проверки квоты.")


def checkpoint(reason):
    value = {"status": "paused", "paused_at": int(time.time()), **reason}
    # Persist the reason before touching Beads, even if task synchronization fails.
    write_json(PAUSED, value)
    for name, args in (("head", ["git", "rev-parse", "HEAD"]),
                       ("working_tree", ["git", "status", "--porcelain"])):
        try:
            result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=15)
            if result.returncode == 0:
                value[name] = result.stdout.strip()
        except (OSError, ValueError, subprocess.TimeoutExpired):
            pass
    try:
        value["tasks"] = block_active_tasks(value)
    except (TaskStateUnavailable, ValueError, TypeError) as exc:
        value["task_state_error"] = str(exc)
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
        resume_paused_tasks()
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
            from workflow_notifications import flush_pending
            try:
                flush_pending(limit=1)
            except (OSError, ValueError):
                print("Telegram delivery will be retried.", flush=True)
            for _ in range(30):
                if process.poll() is not None or requested:
                    break
                time.sleep(0.5)
        return process.returncode
    except (QuotaUnavailable, TaskStateUnavailable, OSError) as exc:
        if read_only:
            print("Quota status is temporarily unavailable; the running workflow is unchanged.", file=sys.stderr)
            return 1
        if process is not None and process.poll() is None:
            stop_workflow(process)
        reason = "task_state_unavailable" if isinstance(exc, TaskStateUnavailable) else "quota_unavailable"
        checkpoint({"reason": reason, "detail": str(exc) if isinstance(exc, (QuotaUnavailable, TaskStateUnavailable)) else "Local quota reader failed"})
        return 1 if isinstance(exc, TaskStateUnavailable) else 0
    finally:
        if client:
            client.close()


if __name__ == "__main__":
    sys.exit(main())
