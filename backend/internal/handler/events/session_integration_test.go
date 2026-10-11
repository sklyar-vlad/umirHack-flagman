package events

import (
	"bufio"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

func TestStreamRechecksRevocationExpiryAndUserActivity(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE sessions SET revoked_at=clock_timestamp()`,
		`UPDATE sessions SET expires_at=clock_timestamp()`,
		`UPDATE users SET is_active=false`,
	} {
		t.Run(mutation, func(t *testing.T) {
			f := newFixture(t)
			server, _, token, _ := f.stream(t, "admin")
			resp := open(t, server, token, "")
			f.exec(t, mutation)
			begin := time.Now()
			if _, err := io.ReadAll(resp.Body); err != nil {
				t.Fatal(err)
			}
			if time.Since(begin) > time.Second {
				t.Fatal("late session closure")
			}
		})
	}
}

func TestStreamRefreshesChangedRoleBeforeFurtherDelivery(t *testing.T) {
	f := newFixture(t)
	server, s, token, _ := f.stream(t, "admin")
	resp := open(t, server, token, "")
	f.exec(t, `UPDATE users SET role='storekeeper'`)
	p := model.OutboxPayload{Topics: []model.EventTopic{model.TopicUsers}}
	f.append(t, s, "users.changed", p)
	p.Topics = []model.EventTopic{model.TopicInventory}
	visible := f.append(t, s, "inventory.changed", p)
	if got := next(t, bufio.NewReader(resp.Body)); got.ID != visible {
		t.Fatal("stale role delivered forbidden event", got)
	}
	// Native EventSource is cookie-authenticated; neither CSRF nor custom headers.
	resp.Body.Close()
	f.exec(t, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1`, authservice.SessionTokenHash(token))
}

func TestDefaultHeartbeatAfterTwentySeconds(t *testing.T) {
	f := newFixture(t)
	server, _, token, cancel := f.stream(t, "manager")
	defer cancel()
	req, _ := http.NewRequest("GET", server.URL+"/api/v1/events", nil)
	req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: token})
	client := &http.Client{Timeout: 25 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == ": heartbeat\n" {
			if elapsed := time.Since(start); elapsed < 19*time.Second || elapsed > 23*time.Second {
				t.Fatal(elapsed)
			}
			return
		}
	}
}
