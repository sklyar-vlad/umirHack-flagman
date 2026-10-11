"""Safety and failure-propagation contracts for the executable demo client."""
import importlib.util
import io
from pathlib import Path
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("demo", Path(__file__).with_name("demo-backend-api.py"))
demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(demo)


class Response(io.BytesIO):
    def __init__(self, body, status=200):
        super().__init__(body)
        self.status = status


class Opener:
    def __init__(self, response):
        self.response = response

    def open(self, request, timeout):
        self.request = request
        return self.response


class DemoContracts(unittest.TestCase):
    def test_rejects_external_or_ambiguous_urls_before_network(self):
        for url in ["http://example.org:8080", "https://127.0.0.1:8080", "http://localhost:8080",
                    "http://127.0.0.1", "http://127.0.0.1:80/api", "http://127.0.0.1:80?q=1",
                    "http://127.0.0.1:80#fragment", "http://user:secret@127.0.0.1:80"]:
            with self.subTest(url=url), self.assertRaises(AssertionError):
                demo.API(url)
        self.assertEqual(demo.loopback("http://127.0.0.1:12345"), "http://127.0.0.1:12345")

    def test_unexpected_status_never_includes_secret_body(self):
        api = demo.API("http://127.0.0.1:12345")
        api.client = Opener(Response(b'{"password":"private-canary"}', 500))
        with self.assertRaisesRegex(AssertionError, "HTTP 500, expected 201") as error:
            api.request("/supplies", 201, {"secret": "private-canary"})
        self.assertNotIn("private-canary", str(error.exception))

    def test_mutations_send_origin_csrf_and_stable_key(self):
        api = demo.API("http://127.0.0.1:12345")
        api.csrf = "test-csrf"
        api.client = Opener(Response(b'{"id":"result"}', 201))
        self.assertEqual(api.request("/orders", 201, {"items": []}, key="same-key"), {"id": "result"})
        self.assertEqual(api.client.request.get_header("Origin"), demo.ORIGIN)
        self.assertEqual(api.client.request.get_header("X-csrf-token"), "test-csrf")
        self.assertEqual(api.client.request.get_header("Idempotency-key"), "same-key")

    def test_missing_and_duplicate_resources_fail(self):
        for items in [[], [{"id": "x"}, {"id": "x"}]]:
            with self.assertRaises(AssertionError):
                demo.find(items, "id", "x")

    def test_response_limit_and_redirects_fail_closed(self):
        api = demo.API("http://127.0.0.1:12345")
        api.client = Opener(Response(b'x' * 2_000_001))
        with self.assertRaisesRegex(AssertionError, "limit"):
            api.request("/inventory")
        self.assertIsNone(demo.NoRedirect().redirect_request(None, None, 302, "", {}, "http://external"))

    def test_sse_comments_and_complete_frame(self):
        response = io.BytesIO(b': heartbeat\n\nid: 12\nevent: inventory.changed\n'
                              b'data: {"topics":["inventory"]}\n\n')
        self.assertEqual(demo.next_event(response), {
            "id": 12, "event": "inventory.changed", "data": {"topics": ["inventory"]}})

    def test_sse_rejects_truncated_invalid_and_oversized_frames(self):
        for body in [b'id: x\n', b'event: inventory.changed\n\n', b'id: 1\n', b'x' * 8193]:
            with self.subTest(size=len(body)), self.assertRaises(AssertionError):
                demo.next_event(io.BytesIO(body))

    def test_owned_command_failure_suppresses_output_and_disallows_wrong_project(self):
        failed = demo.subprocess.CompletedProcess([], 1, "private-canary", "private-canary")
        with patch.object(demo.subprocess, "run", return_value=failed):
            with self.assertRaises(AssertionError) as error:
                demo.command(["test-only"])
            self.assertNotIn("private-canary", str(error.exception))
        with patch.dict(demo.os.environ, {"FLAGMAN_DEMO_PROJECT": "flagman-production", "FLAGMAN_CI_DIR": "/tmp"}):
            with self.assertRaises(AssertionError):
                demo.owned_compose("restart", "backend")

    def test_order_operator_uses_existing_seed_identity(self):
        api = demo.API("http://127.0.0.1:12345")
        with patch.dict(demo.os.environ, {"DEMO_ORDER_OPERATOR_PASSWORD": "test-only"}), \
                patch.object(api, "credentials") as credentials, \
                patch.object(api, "request", return_value={"user": {"role": "order_operator"}}):
            api.login("order_operator")
            credentials.assert_called_once_with("operator@flagman.invalid", "test-only")

    def test_scenario_cannot_target_another_loopback_application(self):
        with patch.object(demo, "owned_compose", return_value="127.0.0.1:12345\n"):
            with self.assertRaisesRegex(AssertionError, "owned Compose"):
                demo.Demo("http://127.0.0.1:8080")


if __name__ == "__main__":
    unittest.main()
