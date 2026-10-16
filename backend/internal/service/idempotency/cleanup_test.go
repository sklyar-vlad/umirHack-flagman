package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
)

type cancellableCleanupRepository struct {
	Repository
	started chan struct{}
}

func (r *cancellableCleanupRepository) WithinTx(
	ctx context.Context, _ *pgx.TxOptions, _ func(repository.DBTX) error,
) error {
	close(r.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestCleanupStopsOnShutdownDuringDatabaseCall(t *testing.T) {
	r := &cancellableCleanupRepository{started: make(chan struct{})}
	s := NewService(r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.RunCleanup(ctx, zap.NewNop()) }()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start immediately")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not stop after shutdown")
	}
}
