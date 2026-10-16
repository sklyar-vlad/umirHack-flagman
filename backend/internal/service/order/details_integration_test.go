package order

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestOrderHistoryIntegrationImmutableDetails(t *testing.T) {
	f := newHistoryFixture(t)
	shared, other := f.product(t, "Saved shared"), f.product(t, "Saved other")
	f.batch(t, shared, "0.400", f.now.Add(time.Hour))
	f.batch(t, shared, "5.000", f.now.Add(2*time.Hour))
	f.batch(t, other, "10.000", f.now.Add(time.Hour))
	first, second := f.dish(t, "Saved first", true), f.dish(t, "Saved second", true)
	f.recipe(t, first, 1, map[uuid.UUID]string{shared: "0.300", other: "0.500"})
	f.recipe(t, second, 3, map[uuid.UUID]string{shared: "0.300"})
	created := createHistoryOrder(
		t,
		f,
		dto.OrderRequestItem{DishID: first.String(), Portions: 1},
		dto.OrderRequestItem{DishID: second.String(), Portions: 1},
	)
	createHistoryOrder(t, f, dto.OrderRequestItem{DishID: first.String(), Portions: 2})
	f.exec(t, `UPDATE dishes SET name=name||' changed',is_active=false`)
	f.recipe(t, first, 2, map[uuid.UUID]string{other: "9.000"})
	f.recipe(t, second, 4, map[uuid.UUID]string{other: "8.000"})
	f.exec(t, `UPDATE products SET name=name||' changed',is_active=false`)
	before := f.snapshot(t)
	response := historyRequest(f, "/orders/"+created.ID.String(), true)
	var details dto.OrderDetail
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &details) != nil {
		t.Fatalf("details: %d %s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(details, created) {
		t.Fatalf("historical details changed:\ngot: %+v\nwant: %+v", details, created)
	}
	count := 0
	for _, item := range details.Items {
		for _, allocation := range item.Allocations {
			count++
			var current string
			if err := f.pool.QueryRow(f.ctx, `SELECT remaining_quantity::text FROM batches WHERE id=$1`, allocation.BatchID).
				Scan(&current); err != nil {
				t.Fatal(err)
			}
			if allocation.ProductID == shared && allocation.BatchRemainingQuantityAfter != "0.000" &&
				current == allocation.BatchRemainingQuantityAfter {
				t.Fatal("fixture must demonstrate historical balance differs from current balance")
			}
		}
	}
	if count != 4 || f.snapshot(t) != before {
		t.Fatalf("allocations missing or history wrote data: %d", count)
	}
}

func TestOrderHistoryIntegrationDetailsAccessAndErrors(t *testing.T) {
	f := newHistoryFixture(t)
	product, dish := f.product(t, "Saved"), f.dish(t, "Saved", true)
	f.batch(t, product, "5.000", f.now.Add(time.Hour))
	f.recipe(t, dish, 1, map[uuid.UUID]string{product: "1.000"})
	created := createHistoryOrder(t, f, dto.OrderRequestItem{DishID: dish.String(), Portions: 1})
	for _, role := range []string{"admin", "manager", "order_operator", "storekeeper"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.actor, role)
		want := http.StatusOK
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		if response := historyRequest(f, "/orders/"+created.ID.String(), true); response.Code != want {
			t.Fatalf("%s access: %d %s", role, response.Code, response.Body.String())
		}
	}
	if response := historyRequest(f, "/orders/invalid?unknown=1", true); response.Code != http.StatusForbidden {
		t.Fatal("access must precede UUID/query validation")
	}
	f.exec(t, `UPDATE users SET role='order_operator' WHERE id=$1`, f.actor)
	for _, scenario := range []struct {
		path string
		code int
	}{
		{uuid.NewString(), 404},
		{"invalid", 422},
		{uuid.NewString() + "?search=1", 422},
		{uuid.NewString() + "?q=%ZZ", 422},
		{strings.ReplaceAll(uuid.NewString(), "-", ""), 422},
	} {
		if response := historyRequest(f, "/orders/"+scenario.path, true); response.Code != scenario.code {
			t.Fatalf("%s: %d %s", scenario.path, response.Code, response.Body.String())
		}
	}
	if response := historyRequest(f, "/orders/invalid", false); response.Code != http.StatusUnauthorized {
		t.Fatal("authentication must precede UUID validation")
	}
	f.exec(t, `UPDATE users SET is_active=false WHERE id=$1`, f.actor)
	if response := historyRequest(f, "/orders/"+created.ID.String(), true); response.Code != http.StatusUnauthorized {
		t.Fatalf("inactive user: %d %s", response.Code, response.Body.String())
	}
}
