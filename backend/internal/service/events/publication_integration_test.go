package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	eventrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
	"go.uber.org/zap"
)

func (f *fixture) service() *Service {
	return NewService(eventrepo.NewRepository(), repository.NewRepository(f.pool, zap.NewNop()), zap.NewNop())
}
func (f *fixture) pending(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO outbox_events(id,event_type,payload,created_at) VALUES($1,'inventory.changed','{"topics":["inventory"]}',clock_timestamp())`, id)
	return id
}
func TestPublicationCommitOrderRestartAndConcurrentDispatch(t *testing.T) {
	f := newFixture(t)
	first, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(f.ctx)
	second, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Rollback(f.ctx)
	writer := repository.NewRepository(f.pool, zap.NewNop())
	for _, tx := range []pgx.Tx{first, second} {
		if err := writer.AppendOutbox(f.ctx, tx, &model.OutboxEvent{Type: model.InventoryTimeChanged}); err != nil {
			t.Fatal(err)
		}
	}
	if err := second.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	s := f.service()
	if err := s.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := f.pool.QueryRow(f.ctx, `SELECT cursor FROM outbox_events`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := first.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Restart: a fresh service recovers the older-created, later-committed row.
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	var reversed bool
	if err := f.pool.QueryRow(f.ctx, `SELECT (array_agg(cursor ORDER BY created_at))[1]>(array_agg(cursor ORDER BY created_at))[2] FROM outbox_events`).Scan(&reversed); err != nil || !reversed {
		t.Fatal("business creation order leaked into tail", err)
	}
	for range 205 {
		f.pending(t)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errs <- f.service().Dispatch(f.ctx) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count, distinct, pending int
	var high, max int64
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*),count(DISTINCT cursor),count(*) FILTER(WHERE published_at IS NULL),max(cursor),(SELECT high_water FROM outbox_publication_state) FROM outbox_events`).Scan(&count, &distinct, &pending, &max, &high); err != nil || count != 207 || distinct != 207 || pending != 0 || high != max || high <= before {
		t.Fatal(count, distinct, pending, high, max, err)
	}
}

func TestPublicationRollsBackPairAndRetriesSequenceGap(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(map[bool]string{true: "deferred", false: "immediate"}[deferred], func(t *testing.T) {
			f := newFixture(t)
			f.pending(t)
			f.exec(t, `CREATE FUNCTION fail_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private database secret'; END $$`)
			if deferred {
				f.exec(t, `CREATE CONSTRAINT TRIGGER fail_publish AFTER UPDATE ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_publish()`)
			} else {
				f.exec(t, `CREATE TRIGGER fail_publish BEFORE UPDATE ON outbox_events FOR EACH ROW EXECUTE FUNCTION fail_publish()`)
			}
			if err := f.service().Dispatch(f.ctx); err == nil {
				t.Fatal("expected publication failure")
			}
			var pending bool
			var high int64
			if err := f.pool.QueryRow(f.ctx, `SELECT cursor IS NULL AND published_at IS NULL,(SELECT high_water FROM outbox_publication_state) FROM outbox_events`).Scan(&pending, &high); err != nil || !pending || high != 0 {
				t.Fatal("partial publication", pending, high, err)
			}
			f.exec(t, `DROP TRIGGER fail_publish ON outbox_events`)
			if err := f.service().Dispatch(f.ctx); err != nil {
				t.Fatal(err)
			}
			var cursor int64
			if err := f.pool.QueryRow(f.ctx, `SELECT cursor FROM outbox_events`).Scan(&cursor); err != nil || cursor <= 1 {
				t.Fatal("sequence gap not retained", cursor, err)
			}
		})
	}
}
func TestDispatcherCancellationDoesNotHoldBusinessWrites(t *testing.T) {
	f := newFixture(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if err := eventrepo.NewRepository().Lock(f.ctx, tx); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 50*time.Millisecond)
	defer cancel()
	if err := f.service().Dispatch(ctx); err == nil {
		t.Fatal("lock wait was not canceled")
	}
	f.pending(t)
}
