package database

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
	"go.uber.org/zap"
)

func TestMetricsSnapshotRealPendingPublicationAndTransactionConflict(t *testing.T) {
	f := newFixture(t)
	pending, age, expiry, err := MetricsSnapshot(f.ctx, f.pool)
	if err != nil || pending != 0 || age != 0 || expiry != 0 {
		t.Fatal(pending, age, expiry, err)
	}
	_, err = f.pool.Exec(
		f.ctx,
		`INSERT INTO outbox_events(event_type,payload,created_at) VALUES('inventory.changed','{}',clock_timestamp()-interval '10 seconds')`,
	)
	if err != nil {
		t.Fatal(err)
	}
	pending, age, _, err = MetricsSnapshot(f.ctx, f.pool)
	if err != nil || pending != 1 || age < 10 {
		t.Fatal(pending, age, err)
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE outbox_events SET cursor=1,published_at=clock_timestamp()`)
	if err != nil {
		t.Fatal(err)
	}
	pending, age, _, err = MetricsSnapshot(f.ctx, f.pool)
	if err != nil || pending != 0 || age != 0 {
		t.Fatal(pending, age, err)
	}
	before := testutil.ToFloat64(telemetry.Default.Conflicts.WithLabelValues("serialization"))
	repo := repository.NewRepository(f.pool, zap.NewNop())
	err = repo.WithinTx(
		f.ctx,
		nil,
		func(repository.DBTX) error { return &pgconn.PgError{Code: "40001", Message: "secret"} },
	)
	if err == nil || testutil.ToFloat64(telemetry.Default.Conflicts.WithLabelValues("serialization")) != before+1 {
		t.Fatal("conflict not recorded")
	}
}

func TestMetricsExpiryLagHonorsEnableAndPositiveBalance(t *testing.T) {
	f := newFixture(t)
	_, err := f.pool.Exec(
		f.ctx,
		`WITH u AS (INSERT INTO users(name,email,password_hash,role) VALUES('Test','metric@example.test','test','admin') RETURNING id),
 c AS (INSERT INTO product_categories(name) VALUES('Test') RETURNING id),
 p AS (INSERT INTO products(category_id,name,unit,display_unit) SELECT id,'Stock','g','g' FROM c RETURNING id),
 s AS (INSERT INTO supplies(number,received_at,created_by) SELECT 'metric',clock_timestamp(),id FROM u RETURNING id)
 INSERT INTO batches(supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at)
 SELECT s.id,p.id,1,1,1,'explicit',clock_timestamp()-interval '60 seconds' FROM s,p`,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, age, err := MetricsSnapshot(f.ctx, f.pool)
	if err != nil || age != 0 {
		t.Fatal(age, err)
	}
	_, err = f.pool.Exec(
		f.ctx,
		`INSERT INTO restaurant_settings(id,restaurant_name,auto_writeoff_expired) VALUES(1,'Test',true)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, age, err = MetricsSnapshot(f.ctx, f.pool)
	if err != nil || age < 60 {
		t.Fatal(age, err)
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE batches SET remaining_quantity=0`)
	if err != nil {
		t.Fatal(err)
	}
	_, _, age, err = MetricsSnapshot(f.ctx, f.pool)
	if err != nil || age != 0 {
		t.Fatal(age, err)
	}
}
