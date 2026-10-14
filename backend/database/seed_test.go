package database

import "testing"

func TestSeedDemoPreservesExistingData(t *testing.T) {
	f := newFixture(t)
	if err := SeedDemo(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	const id = "60000000-0000-4000-8000-000000000001"
	var beforeDate string
	var beforeCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT expires_at::text FROM batches WHERE id=$1`, id).
		Scan(&beforeDate); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE batches SET remaining_quantity=42 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM stock_movements`).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	if err := SeedDemo(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var quantity, afterDate string
	var afterCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text,expires_at::text FROM batches WHERE id=$1`, id).
		Scan(&quantity, &afterDate); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM stock_movements`).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if quantity != "42.000" || afterDate != beforeDate || afterCount != beforeCount {
		t.Fatal("repeated seed changed balances, expiry or movement history")
	}
	var dishes int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM dishes`).Scan(&dishes); err != nil || dishes == 0 {
		t.Fatal("catalog not populated", err)
	}
}
