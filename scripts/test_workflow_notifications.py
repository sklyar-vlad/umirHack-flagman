import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
import urllib.error
from unittest.mock import patch

import workflow_notifications as notifications


class NotificationTests(unittest.TestCase):
    def test_success_is_not_sent_twice(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(notifications, "ROOT", Path(directory)), \
             patch.object(notifications, "credentials", return_value=("test-token", "123")), \
             patch.object(notifications, "send_message", return_value=True) as send:
            issue = {"id": "umi-s09", "title": "S09. Check basket"}
            notifications.notify_completed(issue, "abc123")
            notifications.notify_completed(issue, "abc123")
            send.assert_called_once()
            self.assertIn("abc123", send.call_args.args[0])
            state = json.loads((Path(directory) / ".ralph/notifications.json").read_text())
            self.assertIsNotNone(state[issue["id"]]["sent_at"])

    def test_failure_is_saved_and_retried(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(notifications, "ROOT", Path(directory)), \
             patch.object(notifications, "credentials", return_value=("test-token", "123")), \
             patch.object(notifications, "send_message", side_effect=[False, True]) as send:
            issue = {"id": "umi-s09.1", "title": "S09-T01"}
            notifications.notify_completed(issue)
            state = Path(directory) / ".ralph/notifications.json"
            self.assertIsNone(json.loads(state.read_text())[issue["id"]]["sent_at"])
            notifications.flush_pending()
            self.assertIsNotNone(json.loads(state.read_text())[issue["id"]]["sent_at"])
            self.assertEqual(send.call_count, 2)

    def test_http_error_never_exposes_token(self):
        output = io.StringIO()
        error = urllib.error.HTTPError("https://api.telegram.org/botSECRET/sendMessage", 403, "Forbidden", {}, None)
        with patch.object(notifications, "credentials", return_value=("SECRET", "123")), \
             patch.object(notifications.urllib.request, "urlopen", side_effect=error), contextlib.redirect_stderr(output):
            self.assertFalse(notifications.send_message("message"))
        self.assertIn("403", output.getvalue())
        self.assertNotIn("SECRET", output.getvalue())


if __name__ == "__main__":
    unittest.main()
