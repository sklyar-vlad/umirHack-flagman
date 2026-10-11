package demoevents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestScenariosCreateFreshReceiptsOrdersAndDepletionRefusal(t *testing.T) {
	for _, scenario := range []string{"normal", "depletion", "replenishment", "expiring"} {
		t.Run(scenario, func(t *testing.T) {
			stock := 0
			keys := map[string]bool{}
			mutations := 0
			client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/auth/login":
					serveLogin(t, w, r)
				case "/api/v1/dishes/" + testDish.ID.String():
					dish := testDish
					dish.AvailablePortions = fmt.Sprint(stock)
					_ = json.NewEncoder(w).Encode(dish)
				case "/api/v1/orders/check":
					_ = json.NewEncoder(w).Encode(dto.OrderCheck{CanCreate: stock > 0, Requirements: []dto.Requirement{
						{ProductID: uuid.MustParse("20000000-0000-4000-8000-000000000001"), RequiredQuantity: "0.125", Unit: "g"},
						{ProductID: uuid.MustParse("20000000-0000-4000-8000-000000000002"), RequiredQuantity: "1.000", Unit: "pcs"},
					}})
				case "/api/v1/supplies", "/api/v1/orders":
					requireMutation(t, r)
					key := r.Header.Get("Idempotency-Key")
					if _, err := uuid.Parse(key); err != nil || keys[key] {
						t.Error("invalid/repeated key")
					}
					keys[key] = true
					mutations++
					if r.URL.Path == "/api/v1/supplies" {
						var input dto.CreateSupplyRequest
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Items) != 2 || input.Items[0].Quantity != "0.125" || input.Items[1].Quantity != "1.000" || input.Number != "demo-"+key {
							t.Fatal("invalid supply")
						}
						received, err := time.Parse(time.RFC3339Nano, input.ReceivedAt)
						if err != nil || time.Since(received) > time.Second {
							t.Error("stale receipt date")
						}
						expiry, err := time.Parse(time.RFC3339Nano, *input.Items[0].ExpiresAt)
						ttl := 72 * time.Hour
						if scenario == "expiring" {
							ttl = 30 * time.Second
						}
						if err != nil || expiry.Sub(received) != ttl {
							t.Error("invalid relative expiry")
						}
						stock++
						w.WriteHeader(201)
						_ = json.NewEncoder(w).Encode(dto.SupplyDetail{ID: uuid.New()})
					} else {
						var input dto.OrderRequest
						_ = json.NewDecoder(r.Body).Decode(&input)
						portions := input.Items[0].Portions
						if portions < 1 || portions > 1000 {
							t.Error("invalid portions")
						}
						if stock < portions {
							w.WriteHeader(409)
							_, _ = w.Write([]byte(`{"error":{"code":"INSUFFICIENT_STOCK","message":"private"}}`))
						} else {
							stock -= portions
							w.WriteHeader(201)
							_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
						}
					}
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			if err := client.Login(context.Background(), "test-password"); err != nil {
				t.Fatal(err)
			}
			if scenario == "depletion" {
				stock = 1501
			}
			var out bytes.Buffer
			for range 4 {
				if err := client.Step(context.Background(), testDish.ID.String(), scenario, 30*time.Second, &out); err != nil {
					t.Fatal(err)
				}
			}
			if mutations != 4 || len(keys) != 4 {
				t.Fatal("step did not emit exactly one unique mutation")
			}
			if scenario == "depletion" && (stock != 0 || !strings.Contains(out.String(), "refused reason=INSUFFICIENT_STOCK") || !strings.Contains(out.String(), "portions=1000") || !strings.Contains(out.String(), "portions=501")) {
				t.Fatalf("no depletion: %s", out.String())
			}
			if scenario == "normal" && (strings.Count(out.String(), "order created") != 2 || strings.Count(out.String(), "supply created") != 2) {
				t.Fatalf("no normal flow: %s", out.String())
			}
			if strings.Contains(out.String(), "private") {
				t.Fatal("unsafe output")
			}
		})
	}
}

func TestScenariosRejectInvalidPlanWithoutMutation(t *testing.T) {
	mutations := 0
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path != "/api/v1/orders/check" {
			mutations++
		}
		if strings.Contains(r.URL.Path, "/dishes/") {
			_ = json.NewEncoder(w).Encode(testDish)
		} else {
			_ = json.NewEncoder(w).Encode(dto.OrderCheck{Requirements: []dto.Requirement{{ProductID: uuid.New(), Unit: "pcs", RequiredQuantity: "0.125"}}})
		}
	})
	for _, scenario := range []string{"unknown", "normal", "replenishment", "expiring"} {
		if err := client.Step(context.Background(), testDish.ID.String(), scenario, 30*time.Second, &bytes.Buffer{}); err == nil {
			t.Fatal("accepted invalid plan")
		}
	}
	if mutations != 0 {
		t.Fatal("invalid plan wrote through API")
	}
}
