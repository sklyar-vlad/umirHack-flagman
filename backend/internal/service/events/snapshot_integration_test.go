package events

import (
	"testing"

	"github.com/jackc/pgx/v5"
	eventrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
)

func TestReplayFloorAndEventsShareSnapshotDuringConcurrentCleanup(t *testing.T) {
	f := newFixture(t)
	f.pending(t)
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE outbox_events SET published_at=clock_timestamp()-interval '25 hours'`)
	tx, err := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	var floor int64
	if err := tx.QueryRow(f.ctx, `SELECT retention_floor FROM outbox_publication_state`).
		Scan(&floor); err != nil ||
		floor != 0 {
		t.Fatal(floor, err)
	}
	if err := f.service().Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	old, err := eventrepo.NewRepository().Read(f.ctx, tx, 0)
	if err != nil || old.RetentionFloor != 0 || len(old.Events) != 1 {
		t.Fatal("mixed replay snapshot", old, err)
	}
	fresh, err := f.service().Read(f.ctx, 0)
	if err != nil || fresh.RetentionFloor != 1 || len(fresh.Events) != 0 {
		t.Fatal("cleanup state not durable", fresh, err)
	}
}
