import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("quota_guard", Path(__file__).with_name("quota_guard.py"))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


def quota(used=94, weekly=40):
    return guard.summarize({"rateLimits": {
        "primary": {"usedPercent": used, "windowDurationMins": 300, "resetsAt": 1900000000},
        "secondary": {"usedPercent": weekly, "windowDurationMins": 10080, "resetsAt": 1900000000}}})


class QuotaTests(unittest.TestCase):
    def test_reserve_boundary(self):
        self.assertIsNone(guard.pause_reason(quota(94)))
        self.assertEqual(guard.pause_reason(quota(95))["window"]["remaining_percent"], 5)
        self.assertIsNotNone(guard.pause_reason(quota(100)))

    def test_weekly_limit_also_stops(self):
        self.assertEqual(guard.pause_reason(quota(20, 95))["window"]["window_minutes"], 10080)

    def test_missing_five_hour_and_invalid_usage_fail_closed(self):
        for result in ({}, {"rateLimits": {"primary": {"usedPercent": -1, "windowDurationMins": 300, "resetsAt": 1}}}):
            with self.assertRaises(guard.QuotaUnavailable):
                guard.summarize(result)

    def test_metered_codex_bucket_takes_precedence(self):
        values = {"rateLimits": {"primary": {"usedPercent": 10, "windowDurationMins": 300, "resetsAt": 1}},
                  "rateLimitsByLimitId": {"codex": {"primary": {"usedPercent": 95, "windowDurationMins": 300, "resetsAt": 1}}}}
        self.assertIsNotNone(guard.pause_reason(guard.summarize(values)))

    def test_cannot_start_work_when_at_threshold(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            with (patch.object(sys, "argv", ["quota_guard.py", "--", "do-not-start"]),
                  patch.object(guard, "STATE", state / "quota.json"),
                  patch.object(guard, "PAUSED", state / "paused.json"),
                  patch.object(guard, "checkpoint") as checkpoint,
                  patch.object(guard, "QuotaClient") as client,
                  patch.object(guard.subprocess, "Popen") as launch):
                client.return_value.read.return_value = quota(95)
                self.assertEqual(guard.main(), 0)
                launch.assert_not_called()
                self.assertEqual(checkpoint.call_args.args[0]["reason"], "quota_reserve")

    def test_status_failure_does_not_pause_another_running_workflow(self):
        with (patch.object(sys, "argv", ["quota_guard.py", "--status"]),
              patch.object(guard, "QuotaClient") as client,
              patch.object(guard, "checkpoint") as checkpoint,
              patch.object(guard, "write_json") as write,
              patch.object(guard.subprocess, "Popen") as launch):
            client.return_value.read.side_effect = guard.QuotaUnavailable("timeout")
            self.assertEqual(guard.main(), 1)
            checkpoint.assert_not_called()
            write.assert_not_called()
            launch.assert_not_called()

    def test_status_at_threshold_does_not_change_workflow_state(self):
        with (patch.object(sys, "argv", ["quota_guard.py", "--status"]),
              patch.object(guard, "QuotaClient") as client,
              patch.object(guard, "checkpoint") as checkpoint,
              patch.object(guard, "write_json") as write):
            client.return_value.read.return_value = quota(95)
            self.assertEqual(guard.main(), 0)
            checkpoint.assert_not_called()
            write.assert_not_called()

    def test_quota_crossing_interrupts_and_saves_without_closing_tasks(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            process = subprocess.Popen([sys.executable, "-c", "import time;time.sleep(60)"], start_new_session=True)
            try:
                with (patch.object(sys, "argv", ["quota_guard.py", "--", "workflow"]),
                      patch.object(guard, "STATE", state / "quota.json"),
                      patch.object(guard, "PAUSED", state / "paused.json"),
                      patch.object(guard, "checkpoint") as checkpoint,
                      patch.object(guard, "QuotaClient") as client,
                      patch.object(guard.subprocess, "Popen", return_value=process)):
                    client.return_value.read.side_effect = [quota(94), quota(95)]
                    self.assertEqual(guard.main(), 0)
                    self.assertIsNotNone(process.poll())
                    self.assertEqual(json.loads((state / "paused.json").read_text())["status"], "pausing")
                    self.assertEqual(checkpoint.call_args.args[0]["reason"], "quota_reserve")
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait()


if __name__ == "__main__":
    unittest.main()
