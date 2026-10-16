package database

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(40402)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS goose_db_version (
		id BIGSERIAL PRIMARY KEY, version_id BIGINT NOT NULL,
		is_applied BOOLEAN NOT NULL, tstamp TIMESTAMP NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (version_id) version_id,is_applied
		FROM goose_db_version WHERE version_id>0 ORDER BY version_id,id DESC`)
	if err != nil {
		return err
	}
	applied := map[int64]bool{}
	for rows.Next() {
		var version int64
		var active bool
		if err := rows.Scan(&version, &active); err != nil {
			rows.Close()
			return err
		}
		applied[version] = active
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	for _, name := range files {
		version, err := strconv.ParseInt(strings.SplitN(filepath.Base(name), "_", 2)[0], 10, 64)
		if err != nil || version <= 0 || known[version] {
			return fmt.Errorf("invalid migration version: %s", name)
		}
		known[version] = true
		if applied[version] {
			continue
		}
		data, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		_, afterUp, ok := strings.Cut(string(data), "-- +goose Up")
		if !ok {
			return fmt.Errorf("missing migration Up section: %s", name)
		}
		up, _, _ := strings.Cut(afterUp, "-- +goose Down")
		if _, err := tx.Exec(ctx, up); err != nil {
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`,
			version,
		); err != nil {
			return err
		}
	}
	for version, active := range applied {
		if active && !known[version] {
			return fmt.Errorf("database has unknown migration version %d", version)
		}
	}
	return tx.Commit(ctx)
}
