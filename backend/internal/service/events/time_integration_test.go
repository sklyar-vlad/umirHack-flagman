package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDurableTimeEventConcurrentWorkersAndAtomicFailure(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `CREATE FUNCTION fail_time() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'failure'; END $$`)
	f.exec(t, `CREATE CONSTRAINT TRIGGER fail_time AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_time()`)
	if err := f.service().TimeTick(f.ctx); err == nil {
		t.Fatal("expected rollback")
	}
	var untouched bool
	if err := f.pool.QueryRow(f.ctx, `SELECT last_time_event_at IS NULL AND NOT EXISTS(SELECT 1 FROM outbox_events) FROM outbox_publication_state`).Scan(&untouched); err != nil || !untouched {
		t.Fatal("time watermark survived failed event", err)
	}
	f.exec(t, `DROP TRIGGER fail_time ON outbox_events`)
	for pass := 0; pass < 2; pass++ {
		if pass > 0 {
			f.exec(t, `UPDATE outbox_publication_state SET last_time_event_at=clock_timestamp()-interval '30 seconds'`)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for range 8 {
			wg.Go(func() { errs <- f.service().TimeTick(f.ctx) })
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM outbox_events WHERE event_type='inventory.time_changed' AND actor_id IS NULL AND cursor IS NULL AND payload->'topics' ? 'stop-list'`).Scan(&count); err != nil || count != pass+1 {
			t.Fatal(count, err)
		}
	}
}

func TestRealTimedDispatcherInvalidatesWithoutBusinessMutationAndJoins(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { defer close(done); f.service().Run(ctx) }()
	defer func() { cancel(); <-done }()
	start := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(35 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-timeout.C:
			t.Fatal("time notification exceeded one minute")
		case <-ticker.C:
			var count int
			if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NOT NULL AND event_type='inventory.time_changed'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count >= 2 {
				if time.Since(start) < 29*time.Second {
					t.Fatal("timer repeated too early")
				}
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("dispatcher did not join")
				}
				return
			}
		}
	}
}
