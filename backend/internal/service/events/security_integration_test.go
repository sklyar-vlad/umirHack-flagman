package events

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	eventrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPublicationLogsAndReadErrorsDoNotExposeDatabaseMessages(t *testing.T) {
	f := newFixture(t)
	f.pending(t)
	f.exec(
		t,
		`CREATE FUNCTION secret_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private password cookie session'; END $$`,
	)
	f.exec(
		t,
		`CREATE TRIGGER secret_failure BEFORE UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION secret_failure()`,
	)
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	s := NewService(eventrepo.NewRepository(), repository.NewRepository(f.pool, logger), logger)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-ticker.C:
			if logs.Len() > 0 {
				break loop
			}
		case <-deadline.C:
			cancel()
			<-done
			t.Fatal("missing failure log")
		}
	}
	cancel()
	<-done
	for _, entry := range logs.All() {
		if entry.Message != "outbox dispatch failed" || len(entry.ContextMap()) != 1 ||
			entry.ContextMap()["sqlstate"] != "P0001" {
			t.Fatal(entry)
		}
	}
	f.exec(t, `ALTER TABLE outbox_publication_state RENAME TO hidden_publication_state`)
	_, err := s.Read(f.ctx, 0)
	if err == nil || !strings.Contains(err.Error(), "SQLSTATE 42P01") ||
		strings.Contains(err.Error(), "outbox_publication_state") {
		t.Fatal("unsafe read error", err)
	}
}
