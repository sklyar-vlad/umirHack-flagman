package procurement

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProcurementIntegrationPriorityBeforePagination(t *testing.T) {
	f := newProcurementFixture(t)
	target := "5.000"
	partial := f.product(t, "A partial", &target)
	urgent := f.product(t, "Z urgent", &target)
	empty := f.product(t, "B empty", &target)
	noDishes := f.product(t, "A no dishes", &target)
	d := f.product(t, "D empty", &target)
	e := f.product(t, "E empty", &target)
	for _, id := range []uuid.UUID{partial, urgent, noDishes} {
		f.batch(t, id, "0.500", f.now.Add(time.Hour))
	}
	for i, ingredients := range []map[uuid.UUID]string{{urgent: "1.000", partial: "1.000", empty: "1.000"}, {urgent: "1.000"}} {
		dish := f.dish(t, []string{"Shared", "Other"}[i], true)
		f.recipe(t, dish, 1, ingredients)
	}
	want := []uuid.UUID{urgent, empty, partial, d, e, noDishes}
	var paged []uuid.UUID
	for page := 1; page <= 3; page++ {
		result := f.list(t, "?page="+[]string{"1", "2", "3"}[page-1]+"&page_size=2")
		if result.Total != 6 || result.Page != page || result.PageSize != 2 || len(result.Items) != 2 {
			t.Fatalf("pagination: %+v", result)
		}
		for _, item := range result.Items {
			paged = append(paged, item.ProductID)
		}
	}
	if !slices.Equal(paged, want) {
		t.Fatalf("priority across pages: %v want %v", paged, want)
	}
}

func TestProcurementIntegrationLiteralSearchAndCategory(t *testing.T) {
	f := newProcurementFixture(t)
	target := "5.000"
	literal := f.product(t, `Chicken 50%_\`, &target)
	f.product(t, "Chicken 500XY", &target)
	other := f.product(t, "Other category", &target)
	category := uuid.New()
	f.exec(t, `INSERT INTO product_categories(id,name)VALUES($1,'Other')`, category)
	f.exec(t, `UPDATE products SET category_id=$2 WHERE id=$1`, other, category)
	f.product(t, "Chicken no target", nil)
	dish := f.dish(t, "Dish unrelated to product search", true)
	f.recipe(t, dish, 1, map[uuid.UUID]string{literal: "1.000", other: "1.000"})
	for _, tc := range []struct {
		search string
		want   int64
	}{
		{"50%_\\", 1}, {"%", 1}, {"_", 1}, {`\`, 1}, {"cHICKeN", 2}, {"' OR true --", 0}, {"", 3}, {"Dish unrelated", 0},
	} {
		result := f.list(t, "?search="+url.QueryEscape(tc.search))
		if result.Total != tc.want {
			t.Fatalf("literal search %q: %+v", tc.search, result)
		}
		for _, item := range result.Items {
			if item.ProductID == literal && (len(item.AffectedDishes) != 1 || item.AffectedDishes[0].ID != dish) {
				t.Fatalf("search changed explanation: %+v", item)
			}
		}
	}
	for _, query := range []string{"?category_id=" + category.String(), "?category_id=" + category.String() + "&search=category"} {
		result := f.list(t, query)
		if result.Total != 1 || result.Items[0].ProductID != other || len(result.Items[0].AffectedDishes) != 1 || result.Items[0].AffectedDishes[0].ID != dish {
			t.Fatalf("category: %+v", result)
		}
	}
	for _, query := range []string{"?category_id=" + category.String() + "&search=Chicken", "?category_id=" + uuid.NewString()} {
		if result := f.list(t, query); result.Total != 0 || len(result.Items) != 0 {
			t.Fatalf("empty category: %+v", result)
		}
	}
}
