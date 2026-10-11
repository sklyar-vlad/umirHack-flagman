"""Assert the defense scenario using only an owned disposable loopback API.

Passwords, cookies, CSRF tokens and response bodies never enter console output.
Use Task backend:demo; this client does not accept application/deployment URLs.
"""
import http.cookiejar
import http.client
import base64
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
from decimal import Decimal
from threading import Barrier


ORIGIN = "http://flagman-ci.invalid"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def loopback(base):
    url = urllib.parse.urlsplit(base)
    require(url.scheme == "http" and url.hostname == "127.0.0.1" and url.port
            and not url.path and not url.query and not url.fragment
            and not url.username and not url.password,
            "Demo requires an explicit loopback HTTP endpoint")
    return base


def command(args, **kwargs):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, timeout=90, **kwargs)
    require(result.returncode == 0, "Owned demo command failed (output suppressed)")
    return result.stdout


def owned_compose(*args):
    project = os.environ["FLAGMAN_DEMO_PROJECT"]
    directory = Path(os.environ["FLAGMAN_CI_DIR"]).resolve()
    require(re.fullmatch(r"flagman-ci-[0-9a-f]{12}", project)
            and directory.name.startswith("flagman-ci.") and directory.is_dir(),
            "Missing owned demo resource identity")
    root = Path(__file__).resolve().parent.parent
    return command(["docker", "compose", "--env-file", str(directory / "compose.env"),
                    "-p", project, "-f", str(root / "infrastructure/compose.ci.yaml"),
                    "-f", str(root / "infrastructure/compose.demo.yaml"), *args])


def test_sql(sql):
    # Called only for outbox retention metadata/read-only facts in this owned DB.
    return owned_compose("exec", "-T", "db", "psql", "-X", "-v", "ON_ERROR_STOP=1",
                         "-U", "flagman_ci", "-d", "flagman_ci_test", "-Atc", sql).strip()


def next_event(response):
    frame = {}
    for _ in range(4096):
        line = response.readline(8193)
        require(line and len(line) <= 8192, "Invalid or oversized SSE line")
        line = line.decode().rstrip("\r\n")
        if not line and "event" in frame:
            require("id" in frame and "data" in frame, "Incomplete SSE frame")
            return frame
        if line.startswith("id: "):
            require(line[4:].isdigit(), "Invalid SSE cursor")
            frame["id"] = int(line[4:])
        elif line.startswith("event: "):
            frame["event"] = line[7:]
        elif line.startswith("data: "):
            frame["data"] = json.loads(line[6:])
    raise AssertionError("SSE exceeded frame limit")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, base):
        self.base = loopback(base)
        self.csrf = ""
        self.client = urllib.request.build_opener(
            urllib.request.ProxyHandler({}), NoRedirect(),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def request(self, path, expected=200, data=None, method=None, key=None, headers=None):
        # Respect the ordinary global 10 req/s policy without disabling limits.
        time.sleep(0.12)
        request_headers = {"Origin": ORIGIN, "X-CSRF-Token": self.csrf}
        if data is not None:
            request_headers["Content-Type"] = "application/json"
        if key is not None:
            request_headers["Idempotency-Key"] = key
        request_headers.update(headers or {})
        req = urllib.request.Request(
            self.base + "/api/v1" + path,
            data=None if data is None else json.dumps(data).encode(),
            headers=request_headers, method=method)
        try:
            response = self.client.open(req, timeout=8)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            require(response.status in (expected if isinstance(expected, tuple) else (expected,)),
                    f"{req.get_method()} {path.split('?')[0]}: HTTP {response.status}, expected {expected}")
            body = response.read(2_000_001)
            require(len(body) <= 2_000_000, "Response exceeded demo limit")
            return json.loads(body) if body else None

    def login(self, role):
        email = "operator" if role == "order_operator" else role
        self.credentials(email + "@flagman.invalid", os.environ["DEMO_" + role.upper() + "_PASSWORD"])
        require(self.request("/auth/me")["user"]["role"] == role, "Login role mismatch")

    def credentials(self, email, password):
        auth = self.request("/auth/login", data={"email": email, "password": password})
        self.csrf = auth["csrf_token"]

    def logout(self):
        self.request("/auth/logout", 204, method="POST")
        self.request("/auth/me", 401)

    def event(self, cursor, match=lambda event: True):
        request = urllib.request.Request(self.base + "/api/v1/events",
                                         headers={"Last-Event-ID": str(cursor)})
        with self.client.open(request, timeout=8) as response:
            require(response.status == 200 and response.headers.get_content_type() == "text/event-stream",
                    "SSE response contract mismatch")
            previous, deadline = cursor, time.monotonic() + 45
            for _ in range(1000):
                event = next_event(response)
                require(event["id"] > previous, "SSE cursor must increase")
                previous = event["id"]
                require(set(event["data"]) <= {
                    "occurred_at", "topics", "product_ids", "batch_ids", "dish_ids",
                    "category_ids", "supply_ids", "order_ids", "user_ids"}, "Unsafe SSE data fields")
                if match(event):
                    return event
                require(time.monotonic() < deadline, "SSE target timed out")
        raise AssertionError("SSE target not found")


def wait(check, message, seconds=45):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(0.25)
    raise AssertionError(message)


def timestamp(seconds=0):
    return (datetime.now(timezone.utc) + timedelta(seconds=seconds)).isoformat()


def find(items, key, value):
    matches = [item for item in items if item[key] == value]
    require(len(matches) == 1, f"Expected exactly one matching resource by {key}, got {len(matches)}")
    return matches[0]


class Demo:
    def __init__(self, base):
        require(loopback(base) == "http://" + owned_compose("port", "backend", "8080").strip(),
                "Demo URL must belong to its owned Compose backend")
        self.admin, self.manager = API(base), API(base)
        self.clients = []
        self.base = base
        self.suffix = uuid.uuid4().hex[:10]

    def ready(self):
        def check():
            try:
                self.admin.request("/ready")
                return True
            except (OSError, AssertionError):
                return False
        wait(check, "API readiness timed out")
        self.admin.request("/healthcheck")

    def recipe(self, quantity):
        return [{"product_id": self.product, "quantity_per_portion": quantity}]

    def basket(self, portions=1, dish=None, version=None):
        item = {"dish_id": dish or self.dish, "portions": portions}
        if version is not None:
            item["recipe_version"] = version
        return {"items": [item]}

    def supply(self, quantity, expiry=7200, product=None):
        return self.manager.request("/supplies", 201, {
            "number": "DEMO-" + uuid.uuid4().hex,
            "received_at": timestamp(), "items": [{
                "product_id": product or self.product,
                "quantity": quantity, "expires_at": timestamp(expiry)}]}, key=str(uuid.uuid4()))

    def basic(self):
        self.ready()
        self.admin.request("/auth/me", 401)
        self.admin.login("admin")
        self.manager.login("manager")
        self.clients.extend([self.admin, self.manager])
        category = self.admin.request("/product-categories", 201,
                                      {"name": "Demo " + self.suffix})
        self.category = category["id"]
        product = self.admin.request("/products", 201, {
            "name": "Demo ingredient " + self.suffix, "category_id": category["id"],
            "unit": "g", "display_unit": "g", "target_stock": "500",
            "standard_shelf_life_hours": 24})
        self.product = product["id"]
        self.product_name = product["name"]
        dishes = [self.admin.request("/dishes", 201, {
            "name": f"Demo dish {i} {self.suffix}", "recipe": self.recipe("100")})
            for i in range(2)]
        self.dish, self.other = [dish["id"] for dish in dishes]
        self.old_version = dishes[0]["recipe_version"]
        self.manager.request("/users", 403)
        supply = self.supply("200")
        self.batch = supply["items"][0]["batch"]["id"]
        batches = self.manager.request(f"/products/{self.product}/batches")["items"]
        require(find(batches, "id", self.batch)["remaining_quantity"] == "200.000",
                "Receipt batch balance mismatch")
        require(self.manager.request("/orders/check", data=self.basket())["can_create"],
                "One portion should be available")
        self.order = self.manager.request("/orders", 201, self.basket(), key=str(uuid.uuid4()))
        require(self.order["items"][0]["allocations"][0]["batch_remaining_quantity_after"] == "100.000",
                "Order consumption balance mismatch")
        require(self.manager.request("/orders/" + self.order["id"]) == self.order,
                "Order history differs from saved response")
        find(self.manager.request("/orders")["items"], "id", self.order["id"])
        self.manager.request("/orders", 201, self.basket(), key=str(uuid.uuid4()))
        stop = self.manager.request("/stop-list")["items"]
        require({self.dish, self.other} <= {item["id"] for item in stop},
                "Depleted shared ingredient must stop both dishes")
        recommendation = find(self.manager.request("/procurement-recommendations")["items"],
                              "product_id", self.product)
        require(recommendation["recommended_quantity"] == "500.000",
                "Depleted ingredient procurement mismatch")
        self.supply("200")
        require(not {self.dish, self.other} & {item["id"] for item in self.manager.request("/stop-list")["items"]},
                "Replenishment did not clear stop-list")
        recommendation = find(self.manager.request("/procurement-recommendations")["items"],
                              "product_id", self.product)
        require(recommendation["recommended_quantity"] == "300.000", "Procurement did not recalculate")
        require(self.dish in {item["dish"]["id"] for item in self.manager.request("/hot-offers")["items"]},
                "Expiring stock should generate a hot offer")
        print("PASS basic: login -> supply -> batches -> check -> order -> history -> stop-list -> replenishment -> recommendations")

    def settings(self, **changes):
        settings = self.admin.request("/settings")
        return self.admin.request("/settings", data={
            "expected_revision": settings["revision"], **changes}, method="PATCH")

    def administration(self):
        current = self.admin.request("/dishes/" + self.dish)
        updated = self.admin.request("/dishes/" + self.dish, data={
            "expected_revision": current["revision"], "name": "Demo revised " + self.suffix,
            "recipe": self.recipe("150")}, method="PATCH")
        require(updated["recipe_version"] == self.old_version + 1, "Recipe must append one new version")
        stale = self.manager.request("/orders/check", data=self.basket(version=self.old_version))
        require(not stale["can_create"] and stale["issues"][0]["code"] == "recipe_changed",
                "Old recipe should be refused by check")
        refused = self.manager.request("/orders", 409, self.basket(version=self.old_version), key=str(uuid.uuid4()))
        require(refused["error"]["code"] == "RECIPE_CHANGED", "Old recipe order refusal mismatch")
        require(self.manager.request("/orders/" + self.order["id"]) == self.order,
                "Admin recipe/name edit rewrote historical order")
        new_order = self.manager.request("/orders", 201, self.basket(), key=str(uuid.uuid4()))
        require(new_order["items"][0]["recipe_version"] == updated["recipe_version"]
                and new_order["items"][0]["allocations"][0]["quantity"] == "150.000",
                "New order must consume the new recipe")
        self.settings(expiring_threshold_hours=1)
        require(self.manager.request("/hot-offers")["items"] == [], "Threshold did not remove distant offers")
        self.settings(expiring_threshold_hours=24)
        self.supply("300")
        require(self.dish in {offer["dish"]["id"] for offer in self.manager.request("/hot-offers")["items"]},
                "Threshold/replenishment did not refresh offers")
        roles = self.admin.request("/roles")["items"]
        require({role["code"] for role in roles} == {"admin", "manager", "storekeeper", "order_operator"},
                "Role catalog mismatch")
        email, password = "demo-" + self.suffix + "@flagman.invalid", uuid.uuid4().hex
        user = self.admin.request("/users", 201, {
            "name": "Demo colleague " + self.suffix, "email": email,
            "password": password, "role": "order_operator"})
        colleague = API(self.base)
        colleague.credentials(email, password)
        colleague.request("/orders")
        colleague.request("/supplies", 403)
        user = self.admin.request("/users/" + user["id"], data={
            "expected_revision": user["revision"], "role": "storekeeper"}, method="PATCH")
        colleague.request("/auth/me", 401)
        colleague.credentials(email, password)
        colleague.request("/supplies")
        colleague.request("/orders", 403)
        user = self.admin.request("/users/" + user["id"], data={
            "expected_revision": user["revision"], "is_active": False}, method="PATCH")
        colleague.request("/auth/me", 401)
        require(not self.admin.request("/users/" + user["id"])["is_active"], "User deactivation mismatch")
        audit = self.admin.request("/audit-log?page_size=100")["items"]
        require(any(entry["entity_id"] == self.dish and entry["before"] and entry["after"] for entry in audit),
                "Recipe audit snapshots missing")
        require(any(entry["entity_id"] == user["id"] for entry in audit), "User audit missing")
        self.manager.request("/audit-log", 403)
        print("PASS admin: recipe version/history -> threshold -> roles/revocation -> audit")

    def operations(self):
        self.settings(auto_writeoff_expired=False)
        supplied = self.supply("120")
        batch = supplied["items"][0]["batch"]["id"]
        manual = self.manager.request("/write-offs", 201, {
            "batch_id": batch, "quantity": "40", "reason": "Demo manual loss"}, key=str(uuid.uuid4()))
        require(manual["type"] == "manual_writeoff" and manual["delta_quantity"] == "-40.000"
                and manual["remaining_quantity_after"] == "80.000", "Manual write-off mismatch")
        expired = self.supply("50", expiry=3)["items"][0]["batch"]["id"]
        def expired_batch():
            return find(self.manager.request(f"/products/{self.product}/batches?status=all")["items"], "id", expired)
        wait(lambda: expired_batch()["status"] == "expired", "Batch did not expire")
        require(expired_batch()["remaining_quantity"] == "50.000", "Disabled worker changed expired stock")
        self.manager.request("/write-offs", 201, {
            "batch_id": expired, "quantity": "10", "reason": "Demo expired loss"}, key=str(uuid.uuid4()))
        self.settings(auto_writeoff_expired=True)
        wait(lambda: expired_batch()["remaining_quantity"] == "0.000", "Automatic write-off did not complete", 60)
        self.settings(auto_writeoff_expired=False)
        movements = self.manager.request("/stock-movements?batch_id=" + expired)["items"]
        automatic = find(movements, "type", "expiration")
        require(automatic["delta_quantity"] == "-40.000" and automatic["actor"] is None,
                "Automatic movement facts mismatch")
        losses = find(self.manager.request("/analytics/write-offs")["items"], "product_id", self.product)
        require(losses["manual_written_off_quantity"] == "50.000"
                and losses["expired_written_off_quantity"] == "40.000"
                and losses["total_written_off_quantity"] == "90.000", "Loss report mismatch")
        refusal = self.manager.request("/orders", 409, self.basket(1000), key=str(uuid.uuid4()))
        require(refusal["error"]["code"] == "INSUFFICIENT_STOCK", "Expected real ingredient refusal")
        shortages = find(self.manager.request("/analytics/ingredient-shortages")["items"], "product_id", self.product)
        require(shortages["rejected_attempts_count"] == 1, "Shortage report should count one real refusal")
        forecast = find(self.manager.request("/analytics/stop-list-forecast")["items"], "dish_id", self.dish)
        require(forecast["confidence"] == "low" and forecast["assumptions"]
                and forecast["estimated_stop_at"], "Forecast missing bounded historical estimate")
        print("PASS stock operations: exact manual/automatic losses, shortage report and historical forecast")
        for role in ["storekeeper", "order_operator"]:
            client = API(self.base)
            client.login(role)
            self.clients.append(client)
            client.request("/inventory")
            client.request("/users", 403)
            client.request("/analytics/write-offs", 403)
            client.request("/audit-log", 403)
            client.request("/orders" if role == "storekeeper" else "/supplies", 403)
        self.storekeeper = self.clients[-2]
        self.operator = self.clients[-1]
        first = self.manager.event(0)
        self.before_event = first["id"]
        supply = self.supply("10")
        match = lambda event: supply["id"] in event["data"].get("supply_ids", [])
        delivered = self.manager.event(self.before_event, match)
        require(delivered["event"] == "inventory.changed", "Supply SSE type mismatch")
        require(self.manager.event(self.before_event, match) == delivered, "Reconnect replay changed frame")
        warehouse = self.storekeeper.event(self.before_event, match)
        require("orders" not in warehouse["data"]["topics"] and "order_ids" not in warehouse["data"],
                "Storekeeper stream exposed orders")
        operator = self.operator.event(self.before_event,
                                       lambda event: self.product in event["data"].get("product_ids", []))
        require("supplies" not in operator["data"]["topics"] and "supply_ids" not in operator["data"],
                "Order operator stream exposed supplies")
        self.replayed_event = delivered
        # Accelerate 24-hour retention on one already published test row only.
        # Business stock/order/session/audit data is never rewritten.
        test_sql("UPDATE outbox_events SET published_at=clock_timestamp()-interval '25 hours' "
                 "WHERE cursor=(SELECT min(cursor) FROM outbox_events WHERE published_at IS NOT NULL)")
        wait(lambda: int(test_sql("SELECT retention_floor FROM outbox_publication_state WHERE id=1")) > 0,
             "Retention cleanup did not advance floor")
        require(self.manager.event(0)["event"] == "resync.required", "Old cursor must request resync")
        print("PASS streaming: role-filtered SSE, reconnect replay and retention resync")
        before = self.manager.request("/supplies")["total"]
        generator = command(["go", "-C", str(Path(__file__).resolve().parent.parent / "backend"),
                             "run", "./cmd/demo-events", "--confirm-demo", "--demo-url", self.base,
                             "--origin", ORIGIN, "--scenario", "replenishment", "--count", "2",
                             "--interval", "200ms", "--dish-id", self.other], env={**os.environ, "ENV": "demo"})
        require(generator.count("supply created id=") == 2
                and self.manager.request("/supplies")["total"] == before + 2,
                "Generator must create exactly two ordinary receipts")
        self.monitoring()
        print("PASS operations: manual/automatic write-offs -> losses/shortages/forecast -> role denials -> SSE/replay/resync -> generator -> monitoring")

    def monitoring(self):
        directory = Path(os.environ["FLAGMAN_CI_DIR"])
        token = (directory / "metrics_token").read_text()
        password = (directory / "grafana_admin_password").read_text()
        client = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
        def request(base, path, expected=200, data=None, authorization=None, json_body=True):
            headers = {"Content-Type": "application/json"}
            if authorization:
                headers["Authorization"] = authorization
            req = urllib.request.Request(loopback(base) + path, headers=headers,
                                         data=None if data is None else json.dumps(data).encode())
            try:
                response = client.open(req, timeout=8)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                require(response.status == expected, "Monitoring endpoint status mismatch")
                body = response.read(2_000_001)
                require(len(body) <= 2_000_000, "Monitoring response exceeded limit")
                return json.loads(body) if json_body else body.decode()
        request(self.base, "/metrics", 401, json_body=False)
        request(self.base, "/metrics", 401, authorization="Bearer wrong", json_body=False)
        metrics = request(self.base, "/metrics", authorization="Bearer " + token, json_body=False)
        require("flagman_http_requests_total" in metrics and "flagman_domain_refusals_total" in metrics,
                "Technical metrics missing")
        grafana = loopback(os.environ["FLAGMAN_DEMO_GRAFANA"])
        authorization = "Basic " + base64.b64encode(("admin:" + password).encode()).decode()
        def dashboard_ready():
            try:
                return request(grafana, "/api/dashboards/uid/flagman", authorization=authorization)
            except (OSError, AssertionError):
                return False
        wait(dashboard_ready, "Grafana dashboard provisioning timed out", 60)
        request(grafana, "/api/search", 401)
        source = request(grafana, "/api/datasources/uid/flagman-warehouse", authorization=authorization)
        require((directory / "grafana_pg_password").read_text() not in json.dumps(source), "Grafana exposed source secret")
        query = {"from": "0", "to": "9999999999999", "queries": [{"refId": "A",
                 "datasource": {"uid": "flagman-warehouse", "type": "postgres"},
                 "format": "table", "rawQuery": True,
                 "rawSql": "SELECT product_name,unit,valid_quantity,expired_quantity FROM monitoring_stock"}]}
        result = request(grafana, "/api/ds/query", data=query, authorization=authorization)["results"]["A"]
        require(not result.get("error") and result.get("frames") and self.product_name in json.dumps(result),
                "Grafana warehouse query failed")
        query["queries"][0]["rawSql"] = "SELECT email FROM users"
        forbidden = request(grafana, "/api/ds/query", 400, data=query, authorization=authorization)
        require(forbidden["results"]["A"].get("error"), "Grafana role must not read users")
        def scrape_ready():
            result = request(grafana, "/api/datasources/proxy/uid/flagman-prometheus/api/v1/query?query=up", authorization=authorization)
            return any(sample["metric"].get("job") == "flagman" and sample["value"][1] == "1"
                       for sample in result["data"]["result"])
        wait(scrape_ready, "Prometheus did not scrape protected backend", 45)

    def resilience(self):
        product = self.admin.request("/products", 201, {
            "name": "Demo parallel ingredient " + self.suffix, "category_id": self.category,
            "unit": "pcs", "display_unit": "pcs", "target_stock": "2"})
        dishes = [self.admin.request("/dishes", 201, {
            "name": f"Demo parallel dish {i} {self.suffix}", "recipe": [{
                "product_id": product["id"], "quantity_per_portion": "1"}]}) for i in range(2)]
        supply = self.supply("1", product=product["id"])
        batch = supply["items"][0]["batch"]["id"]
        baskets = [self.basket(dish=dish["id"]) for dish in dishes]
        for basket in baskets:
            require(self.manager.request("/orders/check", data=basket)["can_create"],
                    "Each independent one-portion check must succeed")
        before_attempts = self.manager.request("/order-attempts")["total"]
        combined = self.manager.request("/orders/check", data={"items": [basket["items"][0] for basket in baskets]})
        require(not combined["can_create"] and combined["requirements"][0]["required_quantity"] == "2.000"
                and combined["shortages"][0]["missing_quantity"] == "1.000",
                "Shared ingredient must aggregate basket demand")
        require(self.manager.request("/order-attempts")["total"] == before_attempts,
                "Read-only basket check must not save an attempt")
        peers = [API(self.base), API(self.base)]
        for peer in peers:
            peer.login("manager")
            self.clients.append(peer)
        keys = [str(uuid.uuid4()), str(uuid.uuid4())]
        gate = Barrier(2)
        def create(index):
            gate.wait(timeout=5)
            return peers[index].request("/orders", (201, 409), baskets[index], key=keys[index])
        before_orders = self.manager.request("/orders")["total"]
        with ThreadPoolExecutor(max_workers=2) as executor:
            results = list(executor.map(create, range(2)))
        winners = [i for i, result in enumerate(results) if "id" in result]
        losers = [i for i, result in enumerate(results) if result.get("error", {}).get("code") == "INSUFFICIENT_STOCK"]
        require(len(winners) == 1 and len(losers) == 1, "Parallel orders must yield one success and one stock refusal")
        winner, loser = winners[0], losers[0]
        require(self.manager.request("/orders")["total"] == before_orders + 1
                and self.manager.request("/order-attempts")["total"] == before_attempts + 1,
                "Parallel history/attempt counts mismatch")
        def replay():
            require(peers[winner].request("/orders", 201, baskets[winner], key=keys[winner]) == results[winner],
                    "Success key must replay exact saved response")
            require(peers[loser].request("/orders", 409, baskets[loser], key=keys[loser]) == results[loser],
                    "Refusal key must replay exact saved response")
            require(self.manager.request("/orders")["total"] == before_orders + 1
                    and self.manager.request("/order-attempts")["total"] == before_attempts + 1,
                    "Replay duplicated order or refusal")
        replay()
        changed = self.basket(2, dish=dishes[winner]["id"])
        conflict = peers[winner].request("/orders", 409, changed, key=keys[winner])
        require(conflict["error"]["code"] == "IDEMPOTENCY_CONFLICT", "Changed body must conflict with original key")
        batches = self.manager.request(f"/products/{product['id']}/batches?status=all")["items"]
        require(find(batches, "id", batch)["remaining_quantity"] == "0.000", "Parallel orders overspent batch")
        movements = self.manager.request("/stock-movements?batch_id=" + batch)["items"]
        require(sum(Decimal(item["delta_quantity"]) for item in movements) == 0
                and len([item for item in movements if item["type"] == "order_consumption"]) == 1,
                "Parallel movement reconciliation mismatch")
        stream = self.manager.client.open(urllib.request.Request(self.base + "/api/v1/events", headers={
            "Last-Event-ID": str(self.replayed_event["id"])}), timeout=8)
        try:
            owned_compose("restart", "--timeout", "15", "backend")
            try:
                require(len(stream.read(2_000_001)) <= 2_000_000, "Restart stream did not close within bound")
            except http.client.IncompleteRead:
                pass  # A process stop may terminate chunked transfer mid-frame.
        finally:
            stream.close()
        # Docker may reallocate an ephemeral published port when a container
        # restarts. Discover it from our project rather than probing other APIs.
        address = owned_compose("port", "backend", "8080").strip()
        require(re.fullmatch(r"127\.0\.0\.1:[0-9]+", address), "Unexpected restarted API binding")
        self.base = loopback("http://" + address)
        for client in self.clients:
            client.base = self.base
        self.ready()
        require(self.manager.request("/auth/me")["user"]["role"] == "manager", "Session did not survive restart")
        replay()
        require(self.manager.request("/orders/" + self.order["id"]) == self.order,
                "Historical order did not survive restart")
        require(self.manager.event(self.before_event,
                                  lambda event: event["id"] == self.replayed_event["id"]) == self.replayed_event,
                "Last-Event-ID durable replay changed after restart")
        fresh = self.supply("5")
        event = self.manager.event(self.replayed_event["id"],
                                   lambda event: fresh["id"] in event["data"].get("supply_ids", []))
        require(event["id"] > self.replayed_event["id"], "Post-restart cursor did not advance")
        self.manager.request("/events", 422, headers={"Last-Event-ID": "invalid"})
        print("PASS resilience: shared ingredient shortage -> parallel order/refusal -> exact key replay -> restart/session/history -> durable SSE reconnect")

    def run(self, phase):
        try:
            self.basic()
            if phase != "basic":
                self.administration()
            if phase in {"operations", "all"}:
                self.operations()
            if phase == "all":
                self.resilience()
        except BaseException:
            for client in reversed(self.clients):
                try:
                    client.logout()
                except Exception:
                    pass
            raise
        else:
            for client in reversed(self.clients):
                client.logout()


if __name__ == "__main__":
    try:
        require(len(sys.argv) == 3 and sys.argv[2] in {"basic", "admin", "operations", "all"},
                "Usage: demo-backend-api.py <owned-loopback-url> basic|admin|operations|all")
        Demo(loopback(sys.argv[1])).run(sys.argv[2])
    except AssertionError as error:
        print(f"Backend demo failed: {error}", file=sys.stderr)
        sys.exit(1)
    except Exception as error:
        print(f"Backend demo failed ({type(error).__name__}; transport, response or setup)", file=sys.stderr)
        sys.exit(1)
