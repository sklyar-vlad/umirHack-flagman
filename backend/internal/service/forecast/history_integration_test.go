package forecast

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	forecastrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/forecast"
	"math/big"
	"testing"
	"time"
)

func TestConsumptionHistoricalBoundariesUnitsAndFullPeriod(t *testing.T) {
	f := newFixture(t)
	start := f.now.Add(-7 * 24 * time.Hour)
	f.consumption(t, "0.001", "g", start)
	f.consumption(t, "0.002", "g", f.now.Add(-time.Microsecond))
	f.consumption(t, "11", "g", start.Add(-time.Microsecond))
	f.consumption(t, "12", "g", f.now)
	f.consumption(t, "13", "g", f.now.Add(time.Hour))
	f.consumption(t, "7", "pcs", start)
	f.exec(t, `INSERT INTO stock_movements(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note) VALUES($1,'manual_writeoff',-50,0,'Damage','g',$2,$3,'Damage'),($1,'expiration',-50,0,'Expired','g',NULL,$3,NULL),($1,'receipt',100,100,'Receipt','g',$2,$3,NULL)`, f.batch, f.user, start)
	f.exec(t, `UPDATE products SET name='Current ingredient' WHERE id=$1`, f.product)
	tx, err := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	r := forecastrepo.NewRepository()
	totals, err := r.Consumption(f.ctx, tx, []uuid.UUID{f.product}, f.now, 7)
	if err != nil || len(totals) != 2 {
		t.Fatal(totals, err)
	}
	rates, err := consumptionRates(totals, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Rat).SetFrac(big.NewInt(3), big.NewInt(int64(7*24*time.Hour)))
	if rates[productUnit{f.product, "g"}].Cmp(want) != 0 {
		t.Fatal(totals, rates)
	}
	empty, err := r.Consumption(f.ctx, tx, []uuid.UUID{uuid.New()}, f.now, 7)
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	if _, err := r.Consumption(f.ctx, f.pool, nil, f.now, 7); err == nil {
		t.Fatal("accepted pool")
	}
}
