package supply_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func TestSupplyReadsRequireSessionAndRoleIntegration(t *testing.T) {
	f := newListFixture(t)
	for _, test := range []struct {
		name   string
		role   string
		cookie bool
		status int
		code   string
	}{
		{"no session", "storekeeper", false, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"operator", "order_operator", true, http.StatusForbidden, "FORBIDDEN"},
		{"admin", "admin", true, http.StatusOK, ""},
		{"manager", "manager", true, http.StatusOK, ""},
		{"storekeeper", "storekeeper", true, http.StatusOK, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.pool.Exec(
				f.ctx,
				`UPDATE users SET role=$2 WHERE id=$1`,
				f.actor.String(),
				test.role,
			); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/v1/supplies", "/api/v1/supplies/" + f.ids[1].String()} {
				req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.server.URL+path, http.NoBody)
				if err != nil {
					t.Fatal(err)
				}
				if test.cookie {
					req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: f.token})
				}
				response, err := f.server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != test.status {
					t.Fatalf("%s: status %d, want %d; %s", path, response.StatusCode, test.status, body)
				}
				if test.code != "" {
					var result dto.ErrorResponse
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatal(err)
					}
					if result.Error.Code != test.code || result.RequestID != response.Header.Get("X-Request-ID") {
						t.Fatalf("incorrect access error: %s", body)
					}
					if _, err := uuid.Parse(result.RequestID); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
