package database

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"time"

	appmigration "github.com/J0es1ick/Scheduler/migration"
	"github.com/jmoiron/sqlx"
)

func ApplyMigrations(ctx context.Context, db *sqlx.DB) error {
	lockConn, err := db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer lockConn.Close()
	if _, err = lockConn.ExecContext(ctx,
		`SELECT pg_advisory_lock(hashtext('scheduler_schema_migrations'))`,
	); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = lockConn.ExecContext(unlockCtx,
			`SELECT pg_advisory_unlock(hashtext('scheduler_schema_migrations'))`)
	}()

	// Keep every migration statement on the same dedicated connection that
	// owns the session-level advisory lock. Besides making the lock effective,
	// this also prevents a deadlock when the pool is intentionally limited to
	// a single connection.
	if _, err := lockConn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.Glob(appmigration.Files, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list embedded migrations: %w", err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		var applied bool
		if err = lockConn.GetContext(ctx, &applied,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name,
		); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		if name == "001_init.up.sql" {
			var initialized bool
			if err = lockConn.GetContext(ctx, &initialized, `SELECT to_regclass('public.universities') IS NOT NULL`); err != nil {
				return fmt.Errorf("detect existing schema: %w", err)
			}
			if initialized {
				if _, err = lockConn.ExecContext(ctx,
					`INSERT INTO schema_migrations (name, applied_at) VALUES ($1, $2)`, name, time.Now(),
				); err != nil {
					return fmt.Errorf("baseline migration %s: %w", name, err)
				}
				continue
			}
		}

		sqlBody, err := appmigration.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := lockConn.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, string(sqlBody)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES ($1, $2)`, name, time.Now(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
