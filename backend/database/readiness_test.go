package database

import (
	"context"
	"testing"
	"time"
)

func TestReadyMigrationStateAndMissingSchema(t *testing.T) {
	f := newFixture(t)
	if CheckReady(f.ctx, f.pool) == nil {
		t.Fatal("missing ledger accepted")
	}
	_, err := f.pool.Exec(
		f.ctx,
		`CREATE TABLE goose_db_version(id bigserial primary key,version_id bigint,is_applied bool)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := migrationVersions()
	if err != nil || len(versions) == 0 {
		t.Fatal("manifest missing", err)
	}
	for _, v := range versions {
		if _, err := f.pool.Exec(
			f.ctx,
			`INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`,
			v,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckReady(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	last := versions[len(versions)-1]
	f.pool.Exec(f.ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,false)`, last)
	if CheckReady(f.ctx, f.pool) == nil {
		t.Fatal("rolled back migration accepted")
	}
	f.pool.Exec(f.ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`, last)
	if err := CheckReady(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	f.pool.Exec(f.ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,true)`)
	if CheckReady(f.ctx, f.pool) == nil {
		t.Fatal("future schema accepted")
	}
	f.pool.Exec(f.ctx, `DELETE FROM goose_db_version WHERE version_id=999`)
	f.pool.Exec(f.ctx, `ALTER TABLE outbox_publication_state RENAME TO hidden_state`)
	if CheckReady(f.ctx, f.pool) == nil {
		t.Fatal("missing relation accepted")
	}
}

func TestReadyUnavailableDatabaseAndBoundedWait(t *testing.T) {
	f := newFixture(t)
	// Exhaust every connection, exercising the real pool wait cancellation.
	cfg := f.pool.Config()
	held := make([]func(), 0, cfg.MaxConns)
	for range cfg.MaxConns {
		c, err := f.pool.Acquire(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c.Release)
	}
	defer func() {
		for _, release := range held {
			release()
		}
	}()
	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if CheckReady(ctx, f.pool) == nil || time.Since(started) > time.Second {
		t.Fatal("pool wait not bounded")
	}
	for _, release := range held {
		release()
	}
	held = nil
	f.pool.Close()
	if CheckReady(f.ctx, f.pool) == nil {
		t.Fatal("unavailable pool accepted")
	}
}
