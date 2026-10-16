package events

import (
	"testing"
)

func TestRetentionPublishedPrefixFloorAndRollback(t *testing.T) {
	f := newFixture(t)
	for range 3 {
		f.pending(t)
	}
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE outbox_events SET published_at=clock_timestamp()-interval '25 hours' WHERE cursor IN (1,3)`)

	pending := f.pending(t)
	f.exec(t, `UPDATE outbox_events SET created_at=clock_timestamp()-interval '5 days' WHERE id=$1`, pending)
	f.exec(
		t,
		`CREATE FUNCTION fail_retention() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fail'; END $$`,
	)
	f.exec(
		t,
		`CREATE CONSTRAINT TRIGGER fail_retention AFTER DELETE ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_retention()`,
	)
	if err := f.service().Dispatch(f.ctx); err == nil {
		t.Fatal("expected deferred failure")
	}
	page, err := f.service().Read(f.ctx, 0)
	if err != nil || page.RetentionFloor != 0 || page.HighWater != 3 || len(page.Events) != 3 {
		t.Fatal(page, err)
	}
	f.exec(t, `DROP TRIGGER fail_retention ON outbox_events`)
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	page, err = f.service().Read(f.ctx, 0)
	if err != nil || page.RetentionFloor != 1 || len(page.Events) != 3 || page.Events[0].Cursor != 2 ||
		page.Events[1].Cursor != 3 {
		t.Fatal(page, err)
	}

	f.exec(t, `UPDATE outbox_events SET published_at=clock_timestamp()-interval '25 hours'`)
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	page, err = f.service().Read(f.ctx, 0)
	if err != nil || len(page.Events) != 0 || page.RetentionFloor != page.HighWater || page.HighWater < 4 {
		t.Fatal(page, err)
	}
	high := page.HighWater
	f.pending(t)
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	page, err = f.service().Read(f.ctx, high)
	if err != nil || len(page.Events) != 1 || page.Events[0].Cursor <= high || page.RetentionFloor != high {
		t.Fatal(page, err)
	}
}

func TestReplayReadDoesNotPublishPendingOrModifyFacts(t *testing.T) {
	f := newFixture(t)
	f.pending(t)
	page, err := f.service().Read(f.ctx, 0)
	if err != nil || page.HighWater != 0 || len(page.Events) != 0 {
		t.Fatal(page, err)
	}
	var unpublished bool
	if err := f.pool.QueryRow(f.ctx, `SELECT cursor IS NULL AND published_at IS NULL FROM outbox_events`).
		Scan(&unpublished); err != nil ||
		!unpublished {
		t.Fatal(err)
	}
}
