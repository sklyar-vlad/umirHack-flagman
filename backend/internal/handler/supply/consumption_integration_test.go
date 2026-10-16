package supply_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	supplydto "github.com/sklyar-vlad/umirHack-flagman/internal/dto/supply"
)

func (f *listFixture) newConsumptionSupply(t *testing.T, expires time.Time) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := tx.Exec(f.ctx, `INSERT INTO supplies (id,number,received_at,created_at,created_by)
		VALUES ($1,'CONSUMPTION',$2,$3,$4)`, id.String(), now.Add(-2*time.Hour), now.Add(-time.Hour),
		f.actor.String()); err != nil {
		t.Fatal(err)
	}
	var product uuid.UUID
	if err := tx.QueryRow(f.ctx, `SELECT product_id FROM batches WHERE supply_id=$1 AND line_number=1`,
		f.ids[1].String()).Scan(&product); err != nil {
		t.Fatal(err)
	}
	batchIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for i, quantity := range []string{"3.250", "2.000"} {
		if _, err := tx.Exec(f.ctx, `INSERT INTO batches
			(id,supply_id,product_id,line_number,initial_quantity,remaining_quantity,expiry_source,expires_at,created_at)
			VALUES ($1,$2,$3,$4,$5::numeric,$5::numeric,'explicit',$6,$7)`, batchIDs[i].String(), id.String(),
			product.String(), i+1, quantity, expires, now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(f.ctx, `INSERT INTO stock_movements
		(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at)
		SELECT b.id,'receipt',b.initial_quantity,b.remaining_quantity,p.name,p.unit,$2,b.created_at
		FROM batches b JOIN products p ON p.id=b.product_id WHERE b.supply_id=$1`,
		id.String(), f.actor.String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	return id, batchIDs
}

func (f *listFixture) consumeBatch(t *testing.T, id uuid.UUID, quantity string) {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)

	tag, err := tx.Exec(f.ctx, `WITH consumed AS (
		UPDATE batches SET remaining_quantity=remaining_quantity-$2::numeric
		WHERE id=$1 AND $2::numeric>0 AND remaining_quantity >= $2::numeric
		RETURNING id,product_id,remaining_quantity
	)
	INSERT INTO stock_movements
		(batch_id,type,delta_quantity,remaining_quantity_after,product_name_snapshot,unit_snapshot,actor_id,occurred_at,note)
	SELECT b.id,'manual_writeoff',-$2::numeric,b.remaining_quantity,p.name,p.unit,$3,$4,'Тест расхода'
	FROM consumed b JOIN products p ON p.id=b.product_id`, id.String(), quantity, f.actor.String(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("consumption fixture did not change exactly one batch")
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func (f *listFixture) detail(t *testing.T, id uuid.UUID) *supplydto.SupplyDetailDTO {
	t.Helper()
	status, body := f.getDetail(t, id)
	if status != http.StatusOK {
		t.Fatalf("detail status %d: %s", status, body)
	}
	var result supplydto.SupplyDetailDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func TestSupplyDetailsAfterConsumptionIntegration(t *testing.T) {
	for _, test := range []struct {
		name   string
		hours  int
		status string
	}{
		{"valid", 72, "valid"}, {"expiring", 12, "expiring"}, {"expired fixture", -1, "expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newListFixture(t)
			expires := time.Now().UTC().Truncate(time.Microsecond).Add(time.Duration(test.hours) * time.Hour)
			id, batches := f.newConsumptionSupply(t, expires)
			before := f.detail(t, id)
			if before.ItemsCount != 2 || len(before.Items) != 2 ||
				before.Items[0].Batch.RemainingQuantity != "3.250" || before.Items[1].Batch.RemainingQuantity != "2.000" ||
				before.Items[0].Batch.Status != test.status || before.Items[1].Batch.Status != test.status {
				t.Fatalf("unexpected initial details: %+v", before)
			}
			for _, step := range []struct {
				name      string
				batch     int
				quantity  string
				remaining []string
				statuses  []string
			}{
				{"partial consumption", 0, "1.125", []string{"2.125", "2.000"}, []string{test.status, test.status}},
				{"first batch depleted", 0, "2.125", []string{"0.000", "2.000"}, []string{"depleted", test.status}},
				{"all batches depleted", 1, "2.000", []string{"0.000", "0.000"}, []string{"depleted", "depleted"}},
			} {
				t.Run(step.name, func(t *testing.T) {
					f.consumeBatch(t, batches[step.batch], step.quantity)
					after := f.detail(t, id)
					if !reflect.DeepEqual(before.SupplySummaryDTO, after.SupplySummaryDTO) ||
						len(after.Items) != len(before.Items) {
						t.Fatal("consumption changed the historical supply or removed a line")
					}
					for i := range after.Items {
						item := &after.Items[i]
						if item.Batch.ID != batches[i] || item.Batch.RemainingQuantity != step.remaining[i] ||
							item.Batch.Status != step.statuses[i] {
							t.Fatalf("line %d after %s: %+v", i+1, step.name, item)
						}

						copyItem := *item
						copyItem.Batch.RemainingQuantity = before.Items[i].Batch.RemainingQuantity
						copyItem.Batch.Status = before.Items[i].Batch.Status
						copyItem.Batch.RemainingSeconds = before.Items[i].Batch.RemainingSeconds
						if !reflect.DeepEqual(copyItem, before.Items[i]) {
							t.Fatal("consumption changed original quantity, product, line, dates or actual expiry")
						}
					}
					var mismatches int
					if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM batches b
						WHERE b.supply_id=$1 AND b.remaining_quantity IS DISTINCT FROM
						(SELECT SUM(m.delta_quantity) FROM stock_movements m WHERE m.batch_id=b.id)`,
						id.String()).Scan(&mismatches); err != nil {
						t.Fatal(err)
					}
					if mismatches != 0 {
						t.Fatal("stock movement history does not match batch balances")
					}

					repeated := f.detail(t, id)
					if len(repeated.Items) != len(before.Items) ||
						!reflect.DeepEqual(before.SupplySummaryDTO, repeated.SupplySummaryDTO) {
						t.Fatal("repeated GET lost historical supply details")
					}
					for i := range repeated.Items {
						if repeated.Items[i].Batch.RemainingQuantity != step.remaining[i] ||
							repeated.Items[i].Batch.Status != step.statuses[i] {
							t.Fatal("repeated GET lost the committed consumption")
						}
					}
				})
			}
			list := f.list(t, url.Values{"search": {"CONSUMPTION"}})
			if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != id ||
				list.Items[0].ItemsCount != 2 || list.Items[0].Status != "received" {
				t.Fatal("fully depleted supply disappeared from the list or changed its historical line count")
			}
		})
	}
}
