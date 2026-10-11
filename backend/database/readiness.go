package database

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// CheckReady verifies the deployed binary's complete migration manifest. It does
// not apply migrations and never considers a partially migrated database ready.
func CheckReady(ctx context.Context, pool *pgxpool.Pool) error {
	versions, err := migrationVersions()
	if err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	rows, err := tx.Query(ctx, `SELECT version_id, is_applied FROM
 (SELECT DISTINCT ON (version_id) version_id, is_applied FROM goose_db_version
 ORDER BY version_id, id DESC) v WHERE version_id > 0`)
	if err != nil {
		return err
	}
	applied := make(map[int64]bool)
	for rows.Next() {
		var v int64
		var ok bool
		if err := rows.Scan(&v, &ok); err != nil {
			rows.Close()
			return err
		}
		if ok {
			applied[v] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(applied) != len(versions) {
		return errors.New("schema migration mismatch")
	}
	for _, v := range versions {
		if !applied[v] {
			return errors.New("schema migration mismatch")
		}
	}
	// This also detects a missing critical relation despite a retained Goose ledger.
	var ok bool
	err = tx.QueryRow(ctx, `SELECT bool_and(to_regclass(name) IS NOT NULL) FROM unnest(ARRAY[
 'users','sessions','products','dishes','batches','recipe_versions','recipe_items','stock_movements',
 'orders','order_attempts','restaurant_settings','outbox_events','outbox_publication_state']) name`).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("schema relation missing")
	}
	return tx.Commit(ctx)
}

func migrationVersions() ([]int64, error) {
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	versions := make([]int64, 0, len(names))
	for _, name := range names {
		v, err := strconv.ParseInt(strings.SplitN(filepath.Base(name), "_", 2)[0], 10, 64)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}
