package hotoffers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestOffersIntegrationResponseAndExpiry(t *testing.T) {
	f := newOffersFixture(t)
	product := f.product(t, "Моцарелла")
	expires := f.now.Add(24 * time.Hour)
	batch := f.batch(t, product, "1800.000", expires)
	f.batch(t, product, "999.000", f.now)
	f.batch(t, product, "1800.000", f.now.Add(72*time.Hour))
	dish := f.dish(t, "Пицца «Маргарита»", true, testIngredient{product, "120.000"})
	check := func(score, level string, seconds int64) dto.HotOffer {
		t.Helper()
		result := f.list(t, "")
		if result.Total != 1 || len(result.Items) != 1 {
			t.Fatalf("offers: %+v", result)
		}
		item := result.Items[0]
		if item.Dish.ID != dish || item.Dish.RecipeVersion != 1 || item.Rank != 1 || item.PriorityScore != score ||
			item.PriorityLevel != level ||
			item.AvailablePortions != "30" ||
			len(item.ProblemBatches) != 1 {
			t.Fatalf("offer: %+v", item)
		}
		p := item.ProblemBatches[0]
		if p.BatchID != batch || p.ProductID != product || p.ProductName != "Моцарелла" || p.SupplyID != f.supply ||
			p.SupplyNumber != "S13-1" ||
			p.LineNumber != 1 ||
			p.Unit != "g" ||
			p.RemainingQuantity != "1800.000" ||
			p.UsedForOnePortion != "120.000" ||
			!p.ExpiresAt.Equal(expires) ||
			p.RemainingSeconds != seconds {
			t.Fatalf("batch facts: %+v", p)
		}
		if !strings.Contains(item.Reason, "Моцарелла") || !strings.Contains(item.Reason, "№S13-1, строка 1") ||
			!strings.Contains(item.Reason, "120 г из этой партии") {
			t.Fatal(item.Reason)
		}
		return item
	}
	check("0.0000", "low", 86400)
	f.now = f.now.Add(6 * time.Hour)
	check("1.6667", "low", 64800)
	f.now = expires.Add(-time.Microsecond)
	item := check("6.6667", "high", 0)
	if !strings.Contains(item.Reason, "менее чем через секунду") {
		t.Fatal(item.Reason)
	}
	f.now = expires
	if result := f.list(t, ""); result.Total != 0 || len(result.Items) != 0 {
		t.Fatalf("expired batch still promotes dish: %+v", result)
	}
}

func TestOffersIntegrationPriorityPaginationAndLiteralSearch(t *testing.T) {
	f := newOffersFixture(t)
	p := f.product(t, "Ingredient")
	f.batch(t, p, "100000000.000", f.now.Add(12*time.Hour))
	low := f.dish(t, "Low", true, testIngredient{p, "1.000"})
	high := f.dish(t, "High", true, testIngredient{p, "1.001"})
	a := f.dish(t, `A 100%_\ПИЦЦА`, true, testIngredient{p, "1.000"})
	b := f.dish(t, "B tie", true, testIngredient{p, "1.000"})
	all := f.list(t, "")
	if all.Total != 4 || len(all.Items) != 4 || all.Items[0].Dish.ID != high {
		t.Fatalf("priority: %+v", all)
	}
	for i, item := range all.Items {
		if item.Rank != int64(i+1) || item.PriorityScore != "0.0000" {
			t.Fatalf("exact ranking: %+v", item)
		}
		if i > 1 && all.Items[i-1].Dish.ID.String() > item.Dish.ID.String() {
			t.Fatal("dish IDs must break exact ties")
		}
		page := f.list(t, "?page_size=1&page="+strconv.Itoa(i+1))
		if page.Total != 4 || len(page.Items) != 1 || page.Items[0].Dish.ID != item.Dish.ID ||
			page.Items[0].Rank != item.Rank {
			t.Fatalf("pagination: %+v", page)
		}
	}
	if result := f.list(t, "?page_size=1&page=5"); result.Total != 4 || len(result.Items) != 0 {
		t.Fatalf("empty page: %+v", result)
	}
	for _, tc := range []struct {
		search string
		want   uuid.UUID
	}{{"%", a}, {"_", a}, {`\`, a}, {"пицца", a}, {"low", low}, {"B tie", b}} {
		result := f.list(t, "?search="+url.QueryEscape(tc.search))
		if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Dish.ID != tc.want ||
			result.Items[0].Rank != 1 {
			t.Fatalf("literal %q: %+v", tc.search, result)
		}
	}
	for _, query := range []string{"?search=unknown", "?search=" + url.QueryEscape(`%' OR true --`)} {
		if result := f.list(t, query); result.Total != 0 {
			t.Fatalf("search broadened results: %+v", result)
		}
	}
}

func TestOffersIntegrationSessions(t *testing.T) {
	f := newOffersFixture(t)
	for _, role := range []string{"admin", "manager", "order_operator", "storekeeper"} {
		f.exec(t, `UPDATE users SET role=$1 WHERE id=$2`, role, f.actor)
		response := f.request("?page=0", true)
		want := http.StatusUnprocessableEntity
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("role %s: %d %s", role, response.Code, response.Body.String())
		}
		response = f.request("", true)
		want = http.StatusOK
		if role == "storekeeper" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("role %s: %d", role, response.Code)
		}
	}
	if response := f.request("?page=0", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", response.Code)
	}
	f.exec(t, `UPDATE users SET is_active=false WHERE id=$1`, f.actor)
	if response := f.request("", true); response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user: %d", response.Code)
	}
	f.exec(t, `UPDATE users SET is_active=true,role='manager' WHERE id=$1`, f.actor)
	f.exec(
		t,
		`UPDATE sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1`,
		f.actor,
	)
	if response := f.request("", true); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: %d", response.Code)
	}
}

func TestOffersIntegrationFEFOTiesUseActualBatch(t *testing.T) {
	f := newOffersFixture(t)
	p := f.product(t, "Tie ingredient")
	e := f.now.Add(time.Hour)
	late := f.batch(t, p, "1.000", e)
	early := f.batch(t, p, "1.000", e)

	receipt := uuid.New()
	f.exec(
		t,
		`INSERT INTO supplies(id,number,received_at,created_by)VALUES($1,'Earlier',$2,$3)`,
		receipt,
		f.now.Add(-2*time.Hour),
		f.actor,
	)
	f.exec(t, `UPDATE batches SET supply_id=$1,line_number=1 WHERE id=$2`, receipt, early)
	f.dish(t, "Uses earlier receipt", true, testIngredient{p, "0.500"})
	result := f.list(t, "")
	if len(result.Items) != 1 || len(result.Items[0].ProblemBatches) != 1 ||
		result.Items[0].ProblemBatches[0].BatchID != early ||
		strings.Contains(result.Items[0].Reason, "№S13-1") {
		t.Fatalf("receipt tie: %+v", result)
	}

	f.exec(t, `UPDATE supplies SET received_at=$1 WHERE id=$2`, f.now.Add(-time.Hour), receipt)
	want := late
	if early.String() < late.String() {
		want = early
	}
	result = f.list(t, "")
	if result.Items[0].ProblemBatches[0].BatchID != want {
		t.Fatalf("batch ID tie: %+v", result)
	}
}

func TestOffersIntegrationMultipleIngredientsMaximum(t *testing.T) {
	f := newOffersFixture(t)
	a, b := f.product(t, "A mozzarella"), f.product(t, "B milk")
	f.exec(t, `UPDATE products SET unit='ml',display_unit='l',is_active=false WHERE id=$1`, b)
	first := f.batch(t, a, "3.000", f.now.Add(6*time.Hour))
	second := f.batch(t, b, "5.000", f.now.Add(12*time.Hour))
	f.dish(t, "Dish", true, testIngredient{a, "1.000"}, testIngredient{b, "3.000"})
	result := f.list(t, "")
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("offers: %+v", result)
	}
	item := result.Items[0]
	if item.PriorityScore != "30.0000" || item.PriorityLevel != "high" || item.AvailablePortions != "1" ||
		len(item.ProblemBatches) != 2 ||
		item.ProblemBatches[0].BatchID != first ||
		item.ProblemBatches[1].BatchID != second ||
		item.ProblemBatches[1].UsedForOnePortion != "3.000" ||
		!strings.Contains(item.Reason, "3 мл") {
		t.Fatalf("multiple ingredients: %+v", item)
	}
}
