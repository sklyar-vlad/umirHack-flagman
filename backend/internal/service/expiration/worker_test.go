package expiration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeTransactions struct{}

func (fakeTransactions) WithinTx(ctx context.Context, options *pgx.TxOptions, fn func(repository.DBTX) error) error {
	if options == nil || options.AccessMode != pgx.ReadOnly {
		panic("settings must be read-only")
	}
	return fn(nil)
}

type changingSettings struct {
	mu       sync.Mutex
	values   []bool
	reads    int
	err      error
	observed chan int
}

func (s *changingSettings) Get(ctx context.Context, _ repository.DBTX, lock bool) (model.RestaurantSettings, error) {
	if lock {
		panic("poll must not lock settings")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.reads
	s.reads++
	if s.observed != nil {
		s.observed <- s.reads
	}
	return model.RestaurantSettings{AutoWriteOffExpired: s.values[i%len(s.values)]}, s.err
}

type processorFunc func(context.Context) error

func (f processorFunc) Expire(ctx context.Context) error { return f(ctx) }

func TestWorkerReloadsSettingsAndStops(t *testing.T) {
	settings := &changingSettings{values: []bool{false, true, false, true}, observed: make(chan int, 20)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan struct{}, 20)
	w := NewWorker(settings, fakeTransactions{}, processorFunc(func(ctx context.Context) error { calls <- struct{}{}; return nil }), zap.NewNop())
	done := make(chan struct{})
	go func() { defer close(done); w.run(ctx, 10*time.Millisecond) }()
	for i := 1; i <= 4; i++ {
		select {
		case <-settings.observed:
		case <-time.After(time.Second):
			t.Fatal("no pass")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if len(calls) != 2 {
		t.Fatal("cached settings", len(calls))
	}
	if pollInterval+passTimeout >= time.Minute {
		t.Fatal("expiry cadence exceeds SLA")
	}
}
func TestWorkerCancelsInFlightAndDoesNotStartWhenCancelled(t *testing.T) {
	settings := &changingSettings{values: []bool{true}}
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	w := NewWorker(settings, fakeTransactions{}, processorFunc(func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }), zap.NewNop())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight pass did not cancel")
	}
	w.Run(ctx)
	if settings.reads != 1 {
		t.Fatal("cancelled worker started another pass")
	}
}
func TestWorkerRetriesErrorsWithoutLeakingDetails(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	for _, err := range []error{&pgconn.PgError{Code: "P0001", Message: "secret-value"}, errors.New("secret-value")} {
		settings := &changingSettings{values: []bool{true}, err: err}
		w := NewWorker(settings, fakeTransactions{}, processorFunc(func(context.Context) error { t.Fatal("settings failure processed stock"); return nil }), zap.New(core))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); w.run(ctx, 5*time.Millisecond) }()
		deadline := time.Now().Add(time.Second)
		for logs.Len() < 2 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		cancel()
		<-done
		if logs.Len() < 2 {
			t.Fatal("error was not retried")
		}
		for _, e := range logs.All() {
			if e.Message != "expiry pass failed" || e.ContextMap()["error"] != nil {
				t.Fatal("unsafe error log", e)
			}
		}
		logs.TakeAll()
	}
}
