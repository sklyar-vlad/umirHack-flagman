package database

import "testing"

func TestMigrateFirstAndRepeatedStartup(t *testing.T) {
	f := newEmptyFixture(t)
	if err := Migrate(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	if err := CheckReady(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	if err := SeedDemo(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(
		f.ctx,
		`UPDATE batches SET remaining_quantity=42 WHERE id='60000000-0000-4000-8000-000000000001'`,
	); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var after int
	var remaining string
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM goose_db_version`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id='60000000-0000-4000-8000-000000000001'`).
		Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if before != after || remaining != "42.000" {
		t.Fatal("repeated initialization changed ledger or business data")
	}
}

func TestMigrateFailureRollsBackSchema(t *testing.T) {
	f := newEmptyFixture(t)
	if _, err := f.pool.Exec(f.ctx, `CREATE TABLE users(id integer)`); err != nil {
		t.Fatal(err)
	}
	if Migrate(f.ctx, f.pool) == nil {
		t.Fatal("conflicting existing schema accepted")
	}
	var clean bool
	if err := f.pool.QueryRow(f.ctx, `SELECT to_regclass('goose_db_version') IS NULL AND to_regtype('product_unit') IS NULL`).
		Scan(&clean); err != nil ||
		!clean {
		t.Fatal("failed initialization left partial schema", err)
	}
	if _, err := f.pool.Exec(f.ctx, `DROP TABLE users`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(f.ctx, f.pool); err != nil {
		t.Fatal("initialization could not recover after conflict resolution", err)
	}
}

func TestMigrateConcurrentStartup(t *testing.T) {
	f := newEmptyFixture(t)
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- Migrate(f.ctx, f.pool) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckReady(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
}
