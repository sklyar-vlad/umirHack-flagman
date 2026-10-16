package events

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestReplayReconnectDuplicatesRestartAndRetentionResync(t *testing.T) {
	f := newFixture(t)
	server, s, token, _ := f.stream(t, "manager")
	payload := model.OutboxPayload{OccurredAt: time.Now().UTC(), Topics: []model.EventTopic{model.TopicInventory}}
	ids := []int64{}
	for range 205 {
		ids = append(ids, f.append(t, s, "inventory.changed", &payload))
	}

	if err := s.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		resp := open(t, server, token, strconv.FormatInt(ids[0], 10))
		r := bufio.NewReader(resp.Body)
		for _, want := range ids[1:] {
			if got := next(t, r); got.ID != want {
				t.Fatalf("replay order: got %d want %d", got.ID, want)
			}
		}
		resp.Body.Close()
	}

	f.exec(t, `UPDATE outbox_events SET published_at=clock_timestamp()-interval '25 hours' WHERE cursor=$1`, ids[0])
	if err := s.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	resp := open(t, server, token, "0")
	got := next(t, bufio.NewReader(resp.Body))
	if got.Type != "resync.required" || got.ID != ids[len(ids)-1] {
		t.Fatal(got)
	}
	resp.Body.Close()
	resp = open(t, server, token, strconv.FormatInt(ids[0], 10))
	if got := next(t, bufio.NewReader(resp.Body)); got.ID != ids[1] {
		t.Fatal(got)
	}
	resp.Body.Close()

	resp = open(t, server, token, strconv.FormatInt(ids[len(ids)-1]+1, 10))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatal(resp.StatusCode)
	}
}

func TestCursorValidationBeforeStreaming(t *testing.T) {
	f := newFixture(t)
	server, _, token, _ := f.stream(t, "admin")
	for _, value := range []string{"-1", "+1", "1.0", "1,2", "hello", "9223372036854775808", "9999999999999999999999", "1 2"} {
		resp := open(t, server, token, value)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatal(value, resp.StatusCode, err)
		}
		if resp.Header.Get("Content-Type") == "text/event-stream" {
			t.Fatal("validation after stream headers", string(body))
		}
	}
	for _, values := range [][]string{{""}, {"0", "0"}, {" 1"}, {"1 "}} {
		headers := http.Header{"Last-Event-Id": values}
		if _, _, err := cursor(headers); err == nil {
			t.Fatal(fmt.Sprint(values))
		}
	}
	for _, value := range []string{"0", "0000", "9223372036854775807"} {
		if _, replay, err := cursor(http.Header{"Last-Event-Id": []string{value}}); err != nil || !replay {
			t.Fatal(value, err)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/events", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "flagman_session", Value: token})
	req.Header.Add("Last-Event-ID", "0")
	req.Header.Add("Last-Event-ID", "0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatal(resp.StatusCode)
	}
}
