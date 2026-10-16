package writeoff_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	writeoffrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"go.uber.org/zap"
)

func (f *fixture) orderBody(t *testing.T, quantity string) []byte {
	t.Helper()
	dish, recipe := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name,is_active) VALUES($1,'Test dish',true)`, dish)
	f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, recipe, dish, f.user)
	f.exec(
		t,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,$3)`,
		recipe,
		f.product,
		quantity,
	)
	raw, err := json.Marshal(dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.String(), Portions: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type gateEvents struct {
	writeoffservice.EventWriter
	entered, release chan struct{}
}

func (g *gateEvents) AppendOutbox(ctx context.Context, tx repository.DBTX, event *model.OutboxEvent) error {
	if err := g.EventWriter.AppendOutbox(ctx, tx, event); err != nil {
		return err
	}
	close(g.entered)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type operationResult struct {
	response model.IdempotencyResponse
	err      error
}

func (f *fixture) reconcile(t *testing.T, want string) {
	t.Helper()
	var remaining, delta string
	var invalid int
	err := f.pool.QueryRow(f.ctx, `SELECT b.remaining_quantity::text,COALESCE(sum(m.delta_quantity),0)::text FROM batches b LEFT JOIN stock_movements m ON m.batch_id=b.id WHERE b.id=$1 GROUP BY b.id`, f.batch).
		Scan(&remaining, &delta)
	if err != nil || remaining != want || delta != want {
		t.Fatalf("reconciliation %s %s want %s err %v", remaining, delta, want, err)
	}
	err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM (SELECT remaining_quantity_after,sum(delta_quantity) OVER(ORDER BY occurred_at,id) balance FROM stock_movements WHERE batch_id=$1) m WHERE m.remaining_quantity_after<>m.balance`, f.batch).
		Scan(&invalid)
	if err != nil || invalid != 0 {
		t.Fatalf("historical balances differ from movement sum %d %v", invalid, err)
	}
}

func TestOrderAndWriteOffBothLockOrdersCannotOverspend(t *testing.T) {
	for _, first := range []string{"order", "writeoff"} {
		t.Run(first, func(t *testing.T) {
			f := newFixture(t)
			body := f.orderBody(t, "7")
			base := repository.NewRepository(f.pool, zap.NewNop())
			idem := idempotency.NewService(base)
			gate := &gateEvents{base, make(chan struct{}), make(chan struct{})}
			defer func() {
				select {
				case <-gate.release:
				default:
					close(gate.release)
				}
			}()
			firstResult, secondResult := make(chan operationResult, 1), make(chan operationResult, 1)
			orderEvents, offEvents := writeoffservice.EventWriter(base), writeoffservice.EventWriter(base)
			if first == "order" {
				orderEvents = gate
			} else {
				offEvents = gate
			}
			order := orderservice.NewCreateService(orderrepository.NewRepository(), orderEvents, idem)
			off := writeoffservice.NewService(writeoffrepository.NewRepository(), offEvents, idem)
			runOrder := func(dst chan operationResult) {
				r, err := order.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
				dst <- operationResult{r, err}
			}
			runOff := func(dst chan operationResult) {
				r, err := off.CreateIdempotent(f.actor(), f.user, uuid.New(), f.body("7"))
				dst <- operationResult{r, err}
			}
			if first == "order" {
				go runOrder(firstResult)
			} else {
				go runOff(firstResult)
			}
			select {
			case <-gate.entered:
			case <-f.ctx.Done():
				t.Fatal("first transaction did not reach event gate")
			}
			if first == "order" {
				go runOff(secondResult)
			} else {
				go runOrder(secondResult)
			}
			waitLock(t, f, "%FROM products%FOR UPDATE%")
			close(gate.release)
			a, b := <-firstResult, <-secondResult
			if a.err != nil || b.err != nil || a.response.Status != 201 || b.response.Status != 409 {
				t.Fatalf("serialized outcomes %+v %+v", a, b)
			}
			f.reconcile(t, "3.000")
			if f.counts(t) != [4]int{map[bool]int{true: 1, false: 0}[first == "writeoff"], 1, 1, 2} {
				t.Fatal("writes", f.counts(t))
			}
			var orders, attempts int
			if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM orders),(SELECT count(*) FROM order_attempts)`).
				Scan(&orders, &attempts); err != nil {
				t.Fatal(err)
			}
			if (first == "order" && (orders != 1 || attempts != 0)) ||
				(first == "writeoff" && (orders != 0 || attempts != 1)) {
				t.Fatal("domain histories", orders, attempts)
			}
		})
	}
}

func TestConcurrentOrderWriteOffAndManualWritesReconcile(t *testing.T) {
	for _, kind := range []string{"order_and_writeoff", "two_writeoffs"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			base := repository.NewRepository(f.pool, zap.NewNop())
			idem := idempotency.NewService(base)
			order := orderservice.NewCreateService(orderrepository.NewRepository(), base, idem)
			body := f.orderBody(t, "3")
			start := make(chan struct{})
			results := make(chan operationResult, 2)
			go func() {
				<-start
				r, err := f.writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), f.body("4"))
				results <- operationResult{r, err}
			}()
			go func() {
				<-start
				var r model.IdempotencyResponse
				var err error
				if kind == "order_and_writeoff" {
					r, err = order.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
				} else {
					r, err = f.writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), f.body("3"))
				}
				results <- operationResult{r, err}
			}()
			close(start)
			for range 2 {
				r := <-results
				if r.err != nil || r.response.Status != 201 {
					t.Fatalf("concurrent success %v %v", r.response, r.err)
				}
			}
			f.reconcile(t, "3.000")
			items, total := f.list(t, model.StockMovementFilter{})
			if total != 3 {
				t.Fatal(total)
			}
			if kind == "order_and_writeoff" {
				var orderID *uuid.UUID
				for _, m := range items {
					if m.Type == "order_consumption" {
						orderID = m.OrderID
						if m.OrderItemID == nil || m.DeltaQuantity != "-3.000" {
							t.Fatal("order movement", m)
						}
					}
				}
				if orderID == nil {
					t.Fatal("order link absent")
				}
				orderItems, n := f.list(
					t,
					model.StockMovementFilter{
						OrderID:   orderID,
						ProductID: &f.product,
						BatchID:   &f.batch,
						Type:      "order_consumption",
					},
				)
				if n != 1 || orderItems[0].OrderID == nil || *orderItems[0].OrderID != *orderID {
					t.Fatal("order filter", orderItems, n)
				}
				receipts, n := f.list(t, model.StockMovementFilter{Type: "receipt"})
				if n != 1 || receipts[0].DeltaQuantity != "10.000" || receipts[0].OrderID != nil {
					t.Fatal("receipt", receipts, n)
				}
			}
		})
	}
}

func TestWriteOffFreshBalanceAfterBatchWait(t *testing.T) {
	f := newFixture(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err = tx.Exec(f.ctx, `SELECT id FROM batches WHERE id=$1 FOR UPDATE`, f.batch); err != nil {
		t.Fatal(err)
	}
	results := make(chan operationResult, 1)
	start := time.Now().UTC()
	go func() {
		r, err := f.writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), f.body("2"))
		results <- operationResult{r, err}
	}()
	waitLock(t, f, "%FROM batches%FOR UPDATE%")
	if _, err = tx.Exec(f.ctx, `UPDATE batches SET remaining_quantity=1 WHERE id=$1`, f.batch); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	result := <-results
	if result.err != nil || result.response.Status != 409 || f.balance(t) != "1.000" ||
		f.counts(t) != [4]int{0, 0, 0, 1} {
		t.Fatalf("fresh stock %+v %v", result, f.counts(t))
	}
	r := f.create(t, "1", uuid.New())
	var m dto.StockMovement
	_ = json.Unmarshal(r.Body, &m)
	if m.OccurredAt.Before(start) || m.RemainingQuantityAfter != "0.000" {
		t.Fatal("fresh movement timestamp", m)
	}
}
