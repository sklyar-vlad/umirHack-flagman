package inventory

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestInventoryFiltersUseQuantitiesAndIndependentCountsIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	mixed := f.product(t, "A Моцарелла", "g", stringPtr("10.000"), true)
	expiringBatch := f.batch(t, mixed, "2.000", f.now.Add(time.Hour))
	expiredBatch := f.batch(t, mixed, "1.000", f.now.Add(-time.Hour))
	expiredOnly := f.product(t, "B Сливки", "ml", stringPtr("1.000"), true)
	oldBatch := f.batch(t, expiredOnly, "3.000", f.now.Add(-2*time.Hour))
	normal := f.product(t, "C Молоко", "ml", stringPtr("2.000"), true)
	f.batch(t, normal, "2.000", f.now.Add(72*time.Hour))
	absent := f.product(t, "D Курица", "g", nil, true)
	zeroTarget := f.product(t, "E Яйца", "pcs", stringPtr("0.000"), true)
	foreign := f.product(t, "A Моцарелла другая", "g", stringPtr("10.000"), true)
	foreignExpiring := f.batch(t, foreign, "1.000", f.now.Add(2*time.Hour))
	foreignExpired := f.batch(t, foreign, "4.000", f.now.Add(-3*time.Hour))
	otherCategory := uuid.New()
	f.exec(t, `INSERT INTO product_categories (id,name) VALUES ($1,'Other')`, otherCategory.String())
	f.exec(t, `UPDATE products SET category_id=$2 WHERE id=$1`, foreign.String(), otherCategory.String())

	counts := dto.InventoryCounts{All: 5, Expiring: 1, Expired: 2, OutOfStock: 3, Deficit: 2}
	attention := []uuid.UUID{oldBatch, expiredBatch, expiringBatch}
	for _, test := range []struct {
		name      string
		query     url.Values
		ids       []uuid.UUID
		total     int64
		counts    dto.InventoryCounts
		attention []uuid.UUID
	}{
		{
			name:  "all products and all problems",
			query: url.Values{},
			ids:   []uuid.UUID{mixed, foreign, expiredOnly, normal, absent, zeroTarget}, total: 6,
			counts:    dto.InventoryCounts{All: 6, Expiring: 2, Expired: 3, OutOfStock: 3, Deficit: 3},
			attention: []uuid.UUID{foreignExpired, oldBatch, expiredBatch, expiringBatch, foreignExpiring},
		},
		{
			name: "category", query: url.Values{"category_id": {f.category.String()}},
			ids: []uuid.UUID{mixed, expiredOnly, normal, absent, zeroTarget}, total: 5, counts: counts, attention: attention,
		},
		{
			name: "expired badge is also expiring", query: url.Values{"filter": {"expiring"}, "category_id": {f.category.String()}},
			ids: []uuid.UUID{mixed}, total: 1, counts: counts, attention: attention,
		},
		{
			name: "expired", query: url.Values{"filter": {"expired"}, "category_id": {f.category.String()}},
			ids: []uuid.UUID{mixed, expiredOnly}, total: 2, counts: counts, attention: attention,
		},
		{
			name:  "expired badge can be out of stock",
			query: url.Values{"filter": {"out_of_stock"}, "category_id": {f.category.String()}},
			ids:   []uuid.UUID{expiredOnly, absent, zeroTarget}, total: 3, counts: counts, attention: attention,
		},
		{
			name:  "null and zero target are not deficit",
			query: url.Values{"filter": {"deficit"}, "category_id": {f.category.String()}},
			ids:   []uuid.UUID{mixed, expiredOnly}, total: 2, counts: counts, attention: attention,
		},
		{
			name:  "filter and page do not affect counters or problems",
			query: url.Values{"filter": {"out_of_stock"}, "category_id": {f.category.String()}, "page_size": {"1"}, "page": {"2"}},
			ids:   []uuid.UUID{absent}, total: 3, counts: counts, attention: attention,
		},
		{
			name:  "empty page keeps total and all problems",
			query: url.Values{"category_id": {f.category.String()}, "page_size": {"2"}, "page": {"4"}},
			ids:   []uuid.UUID{}, total: 5, counts: counts, attention: attention,
		},
		{
			name:  "case insensitive Cyrillic search AND category",
			query: url.Values{"search": {"мОцАрЕлЛа"}, "category_id": {f.category.String()}},
			ids:   []uuid.UUID{mixed}, total: 1,
			counts:    dto.InventoryCounts{All: 1, Expiring: 1, Expired: 1, Deficit: 1},
			attention: []uuid.UUID{expiredBatch, expiringBatch},
		},
		{
			name:  "search and another category",
			query: url.Values{"search": {"моцарелла"}, "category_id": {otherCategory.String()}},
			ids:   []uuid.UUID{foreign}, total: 1,
			counts:    dto.InventoryCounts{All: 1, Expiring: 1, Expired: 1, Deficit: 1},
			attention: []uuid.UUID{foreignExpired, foreignExpiring},
		},
		{
			name:  "empty chosen filter keeps scoped counters and problems",
			query: url.Values{"search": {"сливки"}, "category_id": {f.category.String()}, "filter": {"expiring"}},
			ids:   []uuid.UUID{}, total: 0,
			counts:    dto.InventoryCounts{All: 1, Expired: 1, OutOfStock: 1, Deficit: 1},
			attention: []uuid.UUID{oldBatch},
		},
		{
			name:  "search and category with no intersection",
			query: url.Values{"search": {"сливки"}, "category_id": {otherCategory.String()}},
			ids:   []uuid.UUID{}, counts: dto.InventoryCounts{}, attention: []uuid.UUID{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := f.list(t, test.query)
			ids := make([]uuid.UUID, 0, len(result.Items))
			for i := range result.Items {
				item := &result.Items[i]
				ids = append(ids, item.Product.ID)
				if item.Product.ID == mixed &&
					(item.Status != "expired" || !item.HasExpiring || !item.HasExpired || item.IsOutOfStock) {
					t.Fatalf("mixed product lost overlapping flags: %+v", item)
				}
				if item.Product.ID == expiredOnly &&
					(item.Status != "expired" || !item.IsOutOfStock || item.AvailableQuantity != "0.000") {
					t.Fatalf("expired stock must not be available: %+v", item)
				}
			}
			if !reflect.DeepEqual(ids, test.ids) || result.Total != test.total || result.Counts != test.counts {
				t.Fatalf("selection IDs %v, total %d, counts %+v; want IDs %v, total %d, counts %+v",
					ids, result.Total, result.Counts, test.ids, test.total, test.counts)
			}
			batches := make([]uuid.UUID, 0, len(result.AttentionBatches))
			for i := range result.AttentionBatches {
				batches = append(batches, result.AttentionBatches[i].BatchID)
			}
			if !reflect.DeepEqual(batches, test.attention) || result.AttentionTotal != int64(len(test.attention)) {
				t.Fatalf("problem batches %v/%d; want %v", batches, result.AttentionTotal, test.attention)
			}
		})
	}
}

func TestInventoryAttentionUsesBatchExpiryAndRemainingQuantityIntegration(t *testing.T) {
	f := newInventoryFixture(t)
	product := f.product(t, "Mixed", "ml", nil, true)
	f.batch(t, product, "10.000", f.now.Add(48*time.Hour))
	expiring := f.batch(t, product, "2.125", f.now.Add(24*time.Hour))
	atExpiry := f.batch(t, product, "3.000", f.now)
	old := f.batch(t, product, "4.500", f.now.Add(-time.Hour))
	depleted := f.batch(t, product, "5.000", f.now.Add(-2*time.Hour))
	f.consume(t, depleted, "0.000")
	f.consume(t, expiring, "1.125")
	result := f.list(t, url.Values{"filter": {"out_of_stock"}})
	if result.Total != 0 || result.AttentionTotal != 3 || result.Counts.Expired != 1 || result.Counts.Expiring != 1 {
		t.Fatalf("problems are batches, independent of selected product filter: %+v", result)
	}
	for i, test := range []struct {
		id       uuid.UUID
		quantity string
		status   string
		expires  time.Time
		seconds  int64
	}{
		{old, "4.500", "expired", f.now.Add(-time.Hour), -3600},
		{atExpiry, "3.000", "expired", f.now, 0},
		{expiring, "1.125", "expiring", f.now.Add(24 * time.Hour), 86400},
	} {
		if i >= len(result.AttentionBatches) {
			t.Fatal("missing problem batch")
		}
		batch := &result.AttentionBatches[i]
		var supply uuid.UUID
		var number string
		if err := f.pool.QueryRow(f.ctx, `SELECT s.id,s.number FROM batches b JOIN supplies s ON s.id=b.supply_id WHERE b.id=$1`,
			test.id.String()).
			Scan(&supply, &number); err != nil {
			t.Fatal(err)
		}
		if batch.BatchID != test.id || batch.ProductID != product || batch.ProductName != "Mixed" ||
			batch.SupplyID != supply || batch.SupplyNumber != number || batch.LineNumber != 1 || batch.Unit != "ml" ||
			batch.RemainingQuantity != test.quantity || batch.Status != test.status ||
			batch.ExpiresAt != test.expires.Format(time.RFC3339Nano) || batch.RemainingSeconds != test.seconds {
			t.Fatalf("problem batch does not match its own expiry and remaining stock: %+v", batch)
		}
	}
	if len(result.AttentionBatches) != 3 {
		t.Fatal("normal or depleted batches appeared in attention")
	}
}
