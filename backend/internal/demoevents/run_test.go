package demoevents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestRunnerBoundsMutationsAndIntervalsAndStopsOnTechnicalFailure(t *testing.T) {
	for _, mode := range []string{"success", "refusal", "failure"} {
		t.Run(mode, func(t *testing.T) {
			var events []time.Time
			logout := false
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/auth/login":
					serveLogin(t, w, r)
				case "/api/v1/auth/logout":
					logout = true
					w.WriteHeader(http.StatusNoContent)
				case "/api/v1/orders":
					events = append(events, time.Now())
					switch mode {
					case "failure":
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"code":"SERVICE_UNAVAILABLE"}}`))
					case "refusal":
						w.WriteHeader(http.StatusConflict)
						_, _ = w.Write([]byte(`{"error":{"code":"INSUFFICIENT_STOCK"}}`))
					default:
						w.WriteHeader(http.StatusCreated)
						_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
					}
				default:
					dish := testDish
					dish.AvailablePortions = "1"
					_ = json.NewEncoder(w).Encode(dish)
				}
			})
			options := Options{
				DishID:   testDish.ID.String(),
				Scenario: "depletion",
				Count:    3,
				Interval: 20 * time.Millisecond,
				Expiry:   30 * time.Second,
			}
			var out bytes.Buffer
			err := client.Run(context.Background(), "test-password", options, &out)
			if mode == "failure" {
				if err == nil || len(events) != 1 {
					t.Fatalf("technical error continued: %v %d", err, len(events))
				}
			} else {
				if err != nil || len(events) != 3 {
					t.Fatalf("incorrect count: %v %d", err, len(events))
				}
				for i := 1; i < len(events); i++ {
					if events[i].Sub(events[i-1]) < options.Interval {
						t.Fatal("interval skipped")
					}
				}
			}
			if !logout {
				t.Fatal("session not revoked")
			}
		})
	}
}

func TestRunnerCancellationDuringIntervalAndHTTPRevokesSession(t *testing.T) {
	for _, mode := range []string{"interval", "request"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mutations := 0
			logout := false
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/auth/login":
					serveLogin(t, w, r)
				case "/api/v1/auth/logout":
					logout = true
					w.WriteHeader(http.StatusNoContent)
				case "/api/v1/orders":
					mutations++
					if mode == "request" {
						_, _ = io.Copy(io.Discard, r.Body)
						cancel()
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
					cancel()
				default:
					dish := testDish
					dish.AvailablePortions = "1"
					_ = json.NewEncoder(w).Encode(dish)
				}
			})
			started := time.Now()
			err := client.Run(
				ctx,
				"test-password",
				Options{
					DishID:   testDish.ID.String(),
					Scenario: "depletion",
					Count:    100,
					Interval: time.Hour,
					Expiry:   30 * time.Second,
				},
				&bytes.Buffer{},
			)
			if !errors.Is(err, context.Canceled) || mutations != 1 || !logout || time.Since(started) > time.Second {
				t.Fatalf("cancellation failed: %v mutations=%d logout=%v", err, mutations, logout)
			}
		})
	}
}

func TestRunnerRejectsInvalidOptionsBeforeLogin(t *testing.T) {
	contacted := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) { contacted = true })
	valid := Options{Scenario: "normal", Count: 1, Expiry: 30 * time.Second}
	cases := []Options{}
	for _, count := range []int{0, -1, 10001} {
		o := valid
		o.Count = count
		cases = append(cases, o)
	}
	for _, interval := range []time.Duration{-1, 25 * time.Hour} {
		o := valid
		o.Interval = interval
		cases = append(cases, o)
	}
	for _, expiry := range []time.Duration{0, 999 * time.Millisecond, 721 * time.Hour} {
		o := valid
		o.Expiry = expiry
		cases = append(cases, o)
	}
	for _, id := range []string{"../auth/login", uuid.Nil.String()} {
		o := valid
		o.DishID = id
		cases = append(cases, o)
	}
	o := valid
	o.Scenario = "unknown"
	cases = append(cases, o)
	for _, options := range cases {
		if err := client.Run(context.Background(), "test-password", options, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %+v", options)
		}
	}
	if contacted {
		t.Fatal("invalid options accessed API")
	}
	if strings.Contains(safeCode("SECRET_TOKEN"), "SECRET") {
		t.Fatal("unallowlisted code leaked")
	}
}
