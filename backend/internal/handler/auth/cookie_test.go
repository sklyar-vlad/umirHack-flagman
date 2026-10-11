package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func TestSessionCookiePolicy(t *testing.T) {
	for _, origin := range []string{"http://flagman.localhost", "https://flagman.example"} {
		service, err := authservice.NewService(nil, origin)
		if err != nil {
			t.Fatal(err)
		}
		h := NewHandler(service)
		for _, token := range []string{"opaque-token", ""} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "http://api.example/api/v1/auth/login", http.NoBody)
			c.Request.Header.Set("X-Forwarded-Proto", "https")
			expires := time.Now().UTC().Add(authservice.SessionLifetime).Truncate(time.Second)
			h.setSessionCookie(c, token, expires)
			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing cookie")
			}
			cookie := cookies[0]
			if cookie.Name != authservice.SessionCookieName || cookie.Value != token || !cookie.HttpOnly ||
				cookie.SameSite != http.SameSiteLaxMode ||
				cookie.Path != "/api/v1" ||
				cookie.Domain != "" ||
				cookie.Secure != (origin == "https://flagman.example") {
				t.Fatal("unsafe cookie policy")
			}
			if token != "" && (cookie.MaxAge != 43200 || !cookie.Expires.Equal(expires)) {
				t.Fatal("incorrect cookie lifetime")
			}
			if token == "" && (cookie.MaxAge != -1 || !cookie.Expires.Before(time.Now())) {
				t.Fatal("cookie was not deleted")
			}
		}
	}
}
