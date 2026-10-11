package demoevents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

var testDish = dto.Dish{ID: uuid.MustParse("30000000-0000-4000-8000-000000000001"), IsActive: true, RecipeVersion: new(1)}

func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient("demo", server.URL, "http://frontend.test")
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

func serveLogin(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Origin") != "http://frontend.test" || r.Header.Get("Content-Type") != "application/json" {
		t.Error("missing login headers")
	}
	var login dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&login); err != nil || login.Email != demoEmail || login.Password != "test-password" {
		t.Errorf("invalid demo login")
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "private-cookie", Path: "/", HttpOnly: true})
	_ = json.NewEncoder(w).Encode(dto.AuthResponse{User: dto.User{Email: demoEmail, Role: "manager", IsActive: true}, CSRFToken: "private-csrf"})
}

func requireMutation(t *testing.T, r *http.Request) {
	t.Helper()
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value != "private-cookie" || r.Header.Get("X-CSRF-Token") != "private-csrf" || r.Header.Get("Origin") != "http://frontend.test" {
		t.Error("mutation without session, origin or CSRF")
	}
}

func TestDemoLoginOrdersUseSessionAndUniqueKeys(t *testing.T) {
	keys := map[string]bool{}
	loggedOut := false
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			serveLogin(t, w, r)
		case "/api/v1/dishes":
			_ = json.NewEncoder(w).Encode(dto.DishListResponse{Items: []dto.Dish{testDish}, Total: 1})
		case "/api/v1/orders":
			requireMutation(t, r)
			key := r.Header.Get("Idempotency-Key")
			if _, err := uuid.Parse(key); err != nil || keys[key] {
				t.Error("invalid or reused key")
			}
			keys[key] = true
			var input dto.OrderRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Items) != 1 || input.Items[0].DishID != testDish.ID.String() || input.Items[0].Portions != 1 || *input.Items[0].RecipeVersion != 1 {
				t.Error("invalid order")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
		case "/api/v1/auth/logout":
			requireMutation(t, r)
			loggedOut = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	if err := client.Login(ctx, "test-password"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for range 2 {
		if err := client.Order(ctx, "", &out); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || !loggedOut || strings.Count(out.String(), "order created id=") != 2 || strings.Contains(out.String(), "private-") {
		t.Fatalf("unexpected safe output: %s", out.String())
	}
}

func TestClientDistinguishesRefusalsAndTechnicalErrors(t *testing.T) {
	for _, tc := range []struct {
		status  int
		code    string
		refusal bool
	}{
		{409, "INSUFFICIENT_STOCK", true}, {409, "RECIPE_CHANGED", true}, {409, "DISH_UNAVAILABLE", true},
		{409, "IDEMPOTENCY_IN_PROGRESS", false}, {422, "INVALID_INPUT", false}, {500, "INSUFFICIENT_STOCK", false}, {429, "RATE_LIMITED", false},
		{503, "secret\ncredential", false},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.code), func(t *testing.T) {
			client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(dto.ErrorResponse{Error: dto.ErrorBody{Code: tc.code, Message: "private-password", Details: map[string]any{"token": "private-token"}}})
			})
			var result dto.OrderDetail
			err := client.request(context.Background(), http.MethodPost, "/orders", uuid.NewString(), basket(testDish, 1), &result)
			var api *APIError
			if !errors.As(err, &api) || api.Refusal != tc.refusal || strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe or wrong error: %v", err)
			}
		})
	}
}

func TestClientRejectsUnsafeConfigAndRedirects(t *testing.T) {
	for _, tc := range []struct{ env, url, origin string }{
		{"production", "http://localhost", "http://frontend.test"}, {"demo", "", "http://frontend.test"},
		{"demo", "http://user:password@localhost", "http://frontend.test"}, {"demo", "http://localhost/api/v1", "http://frontend.test"},
		{"demo", "http://localhost?token=secret", "http://frontend.test"}, {"demo", "ftp://localhost", "http://frontend.test"},
		{"demo", "http://localhost", ""}, {"demo", "http://localhost", "http://frontend.test/path"},
	} {
		if _, err := NewClient(tc.env, tc.url, tc.origin); err == nil {
			t.Errorf("accepted invalid config %+v", tc)
		}
	}
	contacted := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true }))
	defer destination.Close()
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) })
	if err := client.Login(context.Background(), "test-password"); err == nil || contacted {
		t.Fatal("followed credential-bearing redirect")
	}
}

func TestClientCancellationAndInvalidResponses(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"id":"invalid"}`)) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Order(ctx, testDish.ID.String(), &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("not canceled: %v", err)
	}
	var result dto.OrderDetail
	if err := client.request(context.Background(), http.MethodPost, "/orders", uuid.NewString(), basket(testDish, 1), &result); err == nil {
		t.Fatal("accepted malformed response")
	}
}
