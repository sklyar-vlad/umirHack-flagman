"""Check only the loopback API launched by the disposable Compose stack."""
import http.cookiejar
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check(base, state):
    url = urllib.parse.urlsplit(base)
    if url.scheme != "http" or url.hostname != "127.0.0.1" or not url.port or url.path:
        raise ValueError("Smoke requires an explicit loopback HTTP endpoint")
    origin = "http://flagman-ci.invalid"
    client = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
    )

    def request(path, expected, data=None, headers=None, method=None):
        request_headers = {"Origin": origin}
        request_headers.update(headers or {})
        if data is not None:
            request_headers["Content-Type"] = "application/json"
        req = urllib.request.Request(
            base + "/api/v1" + path,
            data=None if data is None else json.dumps(data).encode(),
            headers=request_headers, method=method,
        )
        try:
            response = client.open(req, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            if response.status != expected:
                raise AssertionError(f"{path}: expected {expected}, got {response.status}")
            content = response.read()
            return json.loads(content) if content else None, response.headers

    deadline = time.monotonic() + 45
    while True:
        try:
            request("/ready", 200 if state == "up" else 503)
            break
        except (OSError, AssertionError):
            if time.monotonic() >= deadline:
                raise AssertionError("Readiness did not reach the expected state") from None
            time.sleep(0.25)
    request("/healthcheck", 200)
    if state == "down":
        print("PostgreSQL stopped: readiness 503, liveness 200")
        return

    request("/auth/me", 401)
    login = {"email": "manager@flagman.invalid", "password": os.environ["DEMO_MANAGER_PASSWORD"]}
    request("/auth/login", 403, login, {"Origin": "http://other.invalid"})
    auth, headers = request("/auth/login", 200, login)
    if headers.get("Access-Control-Allow-Origin") != origin or not auth.get("csrf_token"):
        raise AssertionError("Login CORS/CSRF contract failed")
    me, _ = request("/auth/me", 200)
    if me["user"]["role"] != "manager" or not me.get("settings"):
        raise AssertionError("Authenticated identity or settings missing")
    request("/settings", 200)
    request("/inventory", 200)
    request("/dishes", 200)
    request("/users", 403)
    request("/auth/logout", 403, method="POST")
    request("/auth/logout", 204, headers={"X-CSRF-Token": auth["csrf_token"]}, method="POST")
    request("/auth/me", 401)
    print("Readiness, liveness, Origin/CORS, login, inventory/menu, role/CSRF and logout passed")


if __name__ == "__main__":
    try:
        if len(sys.argv) != 3 or sys.argv[2] not in {"up", "down"}:
            raise ValueError("Usage: check-backend-api.py <loopback-url> up|down")
        check(sys.argv[1], sys.argv[2])
    except AssertionError as error:
        # These messages contain only endpoint names/statuses, never responses.
        print(f"Backend API smoke failed: {error}", file=sys.stderr)
        sys.exit(1)
    except Exception:
        # Never dump response bodies, cookies, passwords or request headers.
        print("Backend API smoke failed", file=sys.stderr)
        sys.exit(1)
