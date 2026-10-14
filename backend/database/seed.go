package database

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed seeds/001_catalog.sql seeds/002_inventory.sql
var demoSeeds embed.FS

// SeedDemo adds missing fixtures; the SQL preserves existing balances and dates.
func SeedDemo(ctx context.Context, pool *pgxpool.Pool) error {
	for _, name := range []string{"seeds/001_catalog.sql", "seeds/002_inventory.sql"} {
		data, err := demoSeeds.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read demo fixture %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply demo fixture %s: %w", name, err)
		}
	}
	return nil
}
