package idempotency

import (
	"context"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"go.uber.org/zap"
)

const cleanupBatchSize = 1000

func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	var deleted int64
	err := s.repo.WithinTx(ctx, nil, func(tx repository.DBTX) error {
		var err error
		deleted, err = s.repo.DeleteExpiredIdempotencyResults(ctx, tx, cleanupBatchSize)
		return err
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *Service) RunCleanup(ctx context.Context, logger *zap.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		deleted, err := s.CleanupExpired(batchCtx)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil:
			logger.Error("idempotency cleanup failed", zap.Error(err))
		case deleted > 0:
			logger.Info("expired idempotency results removed", zap.Int64("count", deleted))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
