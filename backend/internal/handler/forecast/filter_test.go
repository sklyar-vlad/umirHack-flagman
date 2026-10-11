package forecast

import "testing"

func TestForecastFilter(t *testing.T) {
	f, err := parseFilter("")
	if err != nil || f.LookbackDays != 7 || f.HorizonHours != 72 || f.Page != 1 || f.PageSize != 50 {
		t.Fatal(f, err)
	}
	for _, q := range []string{"lookback_days=1&horizon_hours=1", "lookback_days=90&horizon_hours=720&page=2&page_size=100"} {
		if _, err := parseFilter(q); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, q := range []string{"lookback_days=0", "lookback_days=91", "horizon_hours=0", "horizon_hours=721", "lookback_days=", "horizon_hours=1.1", "horizon_hours=no", "lookback_days=1&lookback_days=2", "horizon_hours=1&horizon_hours=1", "sort=id", "search=a", "page=0", "page_size=101", "bad=%XX", "lookback_days=99999999999999999999999"} {
		if _, err := parseFilter(q); err == nil {
			t.Fatal("accepted", q)
		}
	}
}
