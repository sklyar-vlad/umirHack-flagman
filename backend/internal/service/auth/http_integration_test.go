package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func authRouter(f *fixture) http.Handler {
	router := gin.New()
	router.Use(basehandler.RequestID())
	access := authhandler.NewHandler(f.service)
	router.Use(access.CORS())
	basehandler.RegisterAuthRoutes(router.Group("/api/v1"), access)
	return router
}

func authRequest(
	router http.Handler,
	method, path, body, origin string,
	cookie *http.Cookie,
	csrf string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://api.example/api/v1"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	return w
}

func loginBody(f *fixture) string {
	data, _ := json.Marshal(dto.LoginRequest{Email: "manager@example.test", Password: f.password})
	return string(data)
}

func decodeAuth(t *testing.T, w *httptest.ResponseRecorder) dto.AuthResponse {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("auth status %d", w.Code)
	}
	var response dto.AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestAuthHTTPLoginMeLogoutAndOrigin(t *testing.T) {
	f := newFixture(t)
	router := authRouter(f)
	for _, origin := range []string{"", "null", "https://evil.example", "https://flagman.example.evil"} {
		w := authRequest(router, "POST", "/auth/login", loginBody(f), origin, nil, "")
		if w.Code != 403 || len(w.Result().Cookies()) != 0 {
			t.Fatal("untrusted login origin accepted")
		}
	}
	w := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	login := decodeAuth(t, w)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || login.User.ID != f.user || login.User.LastLoginAt == nil ||
		login.Settings != nil ||
		w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Access-Control-Allow-Origin") != "https://flagman.example" ||
		w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("incorrect login response")
	}
	cookie := cookies[0]
	if strings.Contains(w.Body.String(), cookie.Value) || strings.Contains(w.Body.String(), "password") {
		t.Fatal("secret exposed in response")
	}
	me := decodeAuth(t, authRequest(router, "GET", "/auth/me", "", "", cookie, ""))
	if me.CSRFToken != login.CSRFToken || !me.SessionExpiresAt.Equal(login.SessionExpiresAt) || me.Settings == nil ||
		me.Settings.RestaurantName != "Test Restaurant" {
		t.Fatal("me response mismatch")
	}
	for _, scenario := range [][2]string{{"https://flagman.example", ""}, {"", ""}, {"https://evil.example", login.CSRFToken}, {"https://flagman.example", "wrong"}} {
		if w := authRequest(router, "POST", "/auth/logout", "", scenario[0], cookie, scenario[1]); w.Code != 403 {
			t.Fatal("unsafe logout accepted")
		}
	}
	w = authRequest(router, "POST", "/auth/logout", "", "https://flagman.example", cookie, login.CSRFToken)
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not delete cookie")
	}
	if w := authRequest(router, "GET", "/auth/me", "", "", cookie, ""); w.Code != 401 {
		t.Fatal("logged-out session accepted")
	}
}

func TestAuthHTTPLimitsAndInvalidBodies(t *testing.T) {
	f := newFixture(t)
	router := authRouter(f)
	for _, body := range []string{`{`, `null`, `{"email":3}`, `{"email":"manager@example.test","password":"x","extra":true}`, `{"email":"","password":"x"}`, `{"email":"manager@example.test","password":"x"} {}`} {
		w := authRequest(router, "POST", "/auth/login", body, "https://flagman.example", nil, "")
		if w.Code != 400 && w.Code != 422 {
			t.Fatalf("invalid login body status %d", w.Code)
		}
	}
	if w := authRequest(
		router,
		"POST",
		"/auth/login",
		strings.Repeat("x", 4097),
		"https://flagman.example",
		nil,
		"",
	); w.Code != 413 {
		t.Fatal("large login body accepted")
	}
	for i := 0; i < 5; i++ {
		w := authRequest(
			router,
			"POST",
			"/auth/login",
			`{"email":"manager@example.test","password":"wrong"}`,
			"https://flagman.example",
			nil,
			"",
		)
		if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("invalid password status %d", w.Code)
		}
	}
	w := authRequest(router, "POST", "/auth/login", loginBody(f), "https://flagman.example", nil, "")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("login attempt limit missing")
	}
}

func TestAuthCORSPreflightAndDuplicateHeaders(t *testing.T) {
	f := newFixture(t)
	router := authRouter(f)
	for _, origin := range []string{"https://flagman.example", "https://evil.example"} {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", http.NoBody)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token, Idempotency-Key")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if origin == "https://flagman.example" {
			if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin ||
				w.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("allowed preflight failed")
			}
		} else if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS allowed foreign origin")
		}
	}
	_, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"origin", "cookie", "csrf", "preflight-header"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", http.NoBody)
		req.Header.Set("Origin", "https://flagman.example")
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: token})
		session, err := f.service.CurrentSession(f.ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-CSRF-Token", session.CSRFToken)
		switch scenario {
		case "origin":
			req.Header.Add("Origin", "https://flagman.example")
		case "csrf":
			req.Header.Add("X-CSRF-Token", session.CSRFToken)
		case "cookie":
			req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: token})
		case "preflight-header":
			req.Method = "OPTIONS"
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Authorization")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		expected := 403
		if scenario == "cookie" {
			expected = 401
		}
		if w.Code != expected {
			t.Fatalf("%s status %d", scenario, w.Code)
		}
	}
}

func TestAuthCORSCoversEarlyRateAndBodyErrors(t *testing.T) {
	f := newFixture(t)
	access := authhandler.NewHandler(f.service)
	for _, body := range []string{"", strings.Repeat("x", 4097)} {
		router := gin.New()
		router.Use(access.CORS())
		if body == "" {
			router.Use(basehandler.RateLimit(1, 1))
		} else {
			router.Use(basehandler.BodyLimit(4096))
		}
		basehandler.RegisterAuthRoutes(router.Group("/api/v1", access.RequireRouteAccess), access)
		if body == "" {
			authRequest(router, "POST", "/auth/login", `{`, "https://flagman.example", nil, "")
		}
		w := authRequest(router, "POST", "/auth/login", body, "https://flagman.example", nil, "")
		expected := 413
		if body == "" {
			expected = 429
		}
		if w.Code != expected || w.Header().Get("Access-Control-Allow-Origin") != "https://flagman.example" ||
			w.Header().Get("Access-Control-Allow-Credentials") != "true" ||
			w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("early error lacks auth CORS/cache policy")
		}
	}
}
