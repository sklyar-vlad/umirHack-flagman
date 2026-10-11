package events

import (
	"bufio"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

// A blocked socket has no subscriber queue or database transaction attached.
// Its deadline closes the write while publication and another client progress.
type socketWriter struct{ net.Conn }

func (w socketWriter) Header() http.Header { return http.Header{} }
func (w socketWriter) WriteHeader(int)     {}
func (w socketWriter) FlushError() error   { return nil }

func TestSlowClientWriteDeadlineDoesNotBlockPublication(t *testing.T) {
	f := newFixture(t)
	server, s, token, cancel := f.stream(t, "admin")
	defer cancel()
	response := open(t, server, token, "")
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	done := make(chan error, 1)
	start := time.Now()
	go func() { w := socketWriter{left}; done <- write(http.NewResponseController(w), w, "data: blocked\n\n") }()
	cursor := f.append(t, s, "inventory.changed", model.OutboxPayload{OccurredAt: time.Now().UTC(), Topics: []model.EventTopic{model.TopicInventory}})
	if time.Since(start) > time.Second {
		t.Fatal("slow socket blocked publication")
	}
	if next(t, bufio.NewReader(response.Body)).ID != cursor {
		t.Fatal("healthy client missed event")
	}
	select {
	case err := <-done:
		if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
			t.Fatal("expected socket timeout", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("unbounded slow client write")
	}
}
