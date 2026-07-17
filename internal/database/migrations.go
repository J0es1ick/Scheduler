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
	if _, err := db.ExecContext(ctx, `
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
		if err = db.GetContext(ctx, &applied,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name,
		); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		if name == "001_init.up.sql" {
			var initialized bool
			if err = db.GetContext(ctx, &initialized, `SELECT to_regclass('public.universities') IS NOT NULL`); err != nil {
				return fmt.Errorf("detect existing schema: %w", err)
			}
			if initialized {
				if _, err = db.ExecContext(ctx,
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
		tx, err := db.BeginTxx(ctx, nil)
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
