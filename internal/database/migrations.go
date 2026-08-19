package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
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

	if _, err := lockConn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			checksum TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if _, err := lockConn.ExecContext(ctx,
		`ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("upgrade schema_migrations metadata: %w", err)
	}
	var appliedAtType string
	if err = lockConn.GetContext(ctx, &appliedAtType, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='schema_migrations'
		  AND column_name='applied_at'`); err != nil {
		return fmt.Errorf("inspect schema_migrations timestamp: %w", err)
	}
	if appliedAtType == "timestamp without time zone" {
		if _, err = lockConn.ExecContext(ctx, `
			ALTER TABLE schema_migrations
			ALTER COLUMN applied_at TYPE TIMESTAMPTZ
			USING applied_at AT TIME ZONE current_setting('TimeZone')`); err != nil {
			return fmt.Errorf("upgrade schema_migrations timestamp: %w", err)
		}
	}

	entries, err := fs.Glob(appmigration.Files, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list embedded migrations: %w", err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		sqlBody, readErr := appmigration.Files.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", name, readErr)
		}
		checksum := migrationChecksum(sqlBody)
		var storedChecksum string
		err = lockConn.GetContext(ctx, &storedChecksum,
			`SELECT checksum FROM schema_migrations WHERE name=$1`, name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if err == nil {
			if storedChecksum == "" {
				if _, err = lockConn.ExecContext(ctx,
					`UPDATE schema_migrations SET checksum=$2 WHERE name=$1 AND checksum=''`, name, checksum,
				); err != nil {
					return fmt.Errorf("record checksum for legacy migration %s: %w", name, err)
				}
			} else if storedChecksum != checksum {
				return fmt.Errorf(
					"migration %s checksum mismatch: applied migration was modified", name,
				)
			}
			continue
		}

		if name == "001_init.up.sql" {
			initialized, baselineErr := legacyBaselineState(ctx, lockConn)
			if baselineErr != nil {
				return baselineErr
			}
			if initialized {
				if _, err = lockConn.ExecContext(ctx,
					`INSERT INTO schema_migrations (name, applied_at, checksum) VALUES ($1, $2, $3)`,
					name, time.Now(), checksum,
				); err != nil {
					return fmt.Errorf("baseline migration %s: %w", name, err)
				}
				continue
			}
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
			`INSERT INTO schema_migrations (name, applied_at, checksum) VALUES ($1, $2, $3)`,
			name, time.Now(), checksum,
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

func VerifyMigrations(ctx context.Context, db *sqlx.DB) error {
	entries, err := fs.Glob(appmigration.Files, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list embedded migrations: %w", err)
	}
	sort.Strings(entries)

	var applied []struct {
		Name     string `db:"name"`
		Checksum string `db:"checksum"`
	}
	if err = db.SelectContext(ctx, &applied,
		`SELECT name, checksum FROM schema_migrations ORDER BY name`); err != nil {
		return fmt.Errorf("read schema migrations; run scheduler-migrate first: %w", err)
	}
	checksums := make(map[string]string, len(applied))
	for _, migration := range applied {
		checksums[migration.Name] = migration.Checksum
	}
	expectedNames := make(map[string]struct{}, len(entries))
	for _, name := range entries {
		expectedNames[name] = struct{}{}
		body, readErr := appmigration.Files.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", name, readErr)
		}
		stored, ok := checksums[name]
		if !ok {
			return fmt.Errorf("migration %s is not applied; run scheduler-migrate first", name)
		}
		expected := migrationChecksum(body)
		if stored != expected {
			return fmt.Errorf("migration %s checksum mismatch: run scheduler-migrate and verify release artifacts", name)
		}
	}
	for _, migration := range applied {
		if _, ok := expectedNames[migration.Name]; !ok {
			return fmt.Errorf(
				"database contains migration %s that is not present in this release; refusing an unsafe downgrade",
				migration.Name,
			)
		}
	}
	return nil
}

type migrationConnection interface {
	GetContext(context.Context, any, string, ...any) error
}

func legacyBaselineState(ctx context.Context, connection migrationConnection) (bool, error) {
	expected := []string{
		"universities", "groups", "semesters", "lessons", "users",
		"subscriptions", "data_sources", "parse_logs",
	}
	var existing int
	if err := connection.GetContext(ctx, &existing, `
		SELECT COUNT(*)::int
		FROM unnest($1::text[]) AS expected(name)
		WHERE to_regclass(format('%I.%I', current_schema(), expected.name)) IS NOT NULL`, expected); err != nil {
		return false, fmt.Errorf("detect existing schema: %w", err)
	}
	if existing == 0 {
		return false, nil
	}
	if existing != len(expected) {
		return false, fmt.Errorf(
			"legacy schema is incomplete: found %d of %d migration 001 tables; refusing to baseline",
			existing, len(expected),
		)
	}
	return true, nil
}

func migrationChecksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
