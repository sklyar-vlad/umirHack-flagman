package order

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func TestOrderCreateIntegrationAuthorizationValidationAndReplay(t *testing.T) {
	f := newCreationFixture(t)
	product, dish := f.product(t, "Ingredient"), f.dish(t, "Dish", true)
	f.batch(t, product, "20.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	body := orderBody(t, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	key := uuid.New()
	for _, scenario := range []struct {
		name, role, origin, csrf, body, path, key, content string
		cookie                                             bool
		duplicate                                          string
		status                                             int
	}{
		{name: "unauthenticated", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), status: 401},
		{name: "storekeeper", role: "storekeeper", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), cookie: true, status: 403},
		{name: "missing csrf", role: "order_operator", origin: "http://flagman.localhost", body: body, key: key.String(), cookie: true, status: 403},
		{name: "wrong origin", role: "order_operator", origin: "http://evil.test", csrf: "csrf", body: body, key: key.String(), cookie: true, status: 403},
		{name: "duplicate origin", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), cookie: true, duplicate: "Origin", status: 403},
		{name: "duplicate csrf", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), cookie: true, duplicate: "X-CSRF-Token", status: 403},
		{name: "missing key", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, cookie: true, status: 422},
		{name: "duplicate key", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), cookie: true, duplicate: "Idempotency-Key", status: 422},
		{name: "nil key", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: uuid.Nil.String(), cookie: true, status: 422},
		{name: "query", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, path: "/api/v1/orders?unknown=1", key: key.String(), cookie: true, status: 422},
		{name: "content type", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: body, key: key.String(), content: "text/plain", cookie: true, status: 400},
		{name: "malformed json", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: "{", key: key.String(), cookie: true, status: 400},
		{name: "empty items", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: `{"items":[]}`, key: key.String(), cookie: true, status: 422},
		{name: "body limit", role: "order_operator", origin: "http://flagman.localhost", csrf: "csrf", body: strings.Repeat(" ", 1<<20) + body, key: key.String(), cookie: true, status: 413},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f.exec(t, `UPDATE users SET role=$1 WHERE id=$2`, scenario.role, f.actor)
			path := scenario.path
			if path == "" {
				path = "/api/v1/orders"
			}
			content := scenario.content
			if content == "" {
				content = "application/json"
			}
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(scenario.body)).WithContext(f.ctx)
			request.Header.Set("Content-Type", content)
			if scenario.origin != "" {
				request.Header.Set("Origin", scenario.origin)
			}
			if scenario.csrf != "" {
				request.Header.Set("X-CSRF-Token", scenario.csrf)
			}
			if scenario.key != "" {
				request.Header.Set("Idempotency-Key", scenario.key)
			}
			if scenario.duplicate != "" {
				request.Header.Add(scenario.duplicate, request.Header.Get(scenario.duplicate))
			}
			if scenario.cookie {
				request.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
			}
			before := f.snapshot(t)
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, request)
			if response.Code != scenario.status || f.snapshot(t) != before {
				t.Fatalf("early refusal: %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, role := range []string{"admin", "manager", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$1 WHERE id=$2`, role, f.actor)
		response := f.createRequest(body, uuid.New())
		if response.Code != http.StatusCreated {
			t.Fatalf("%s cannot create: %d %s", role, response.Code, response.Body.String())
		}
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	first := f.createRequest(body, key)
	if first.Code != http.StatusCreated {
		t.Fatalf("early errors consumed key: %d %s", first.Code, first.Body.String())
	}
	f.exec(t, `UPDATE users SET role='storekeeper' WHERE id=$1`, f.actor)
	before := f.snapshot(t)
	replay := f.createRequest(body, key)
	if replay.Code != http.StatusForbidden || f.snapshot(t) != before {
		t.Fatal("replay bypassed current authorization")
	}
}
