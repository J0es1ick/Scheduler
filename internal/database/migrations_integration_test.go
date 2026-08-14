//go:build integration

package database

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	appmigration "github.com/J0es1ick/Scheduler/migration"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestApplyMigrationsWithSingleConnection(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	db, err := sqlx.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations with one connection: %v", err)
	}
}

func TestApplyMigrationsRejectsChecksumMismatch(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, cleanup := isolatedMigrationSchema(t, ctx, databaseURL)
	defer cleanup()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("initial migration: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE schema_migrations SET checksum='tampered' WHERE name='001_init.up.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("modified migration was not rejected: %v", err)
	}
}

func TestApplyMigrationsRejectsPartialLegacyBaseline(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, cleanup := isolatedMigrationSchema(t, ctx, databaseURL)
	defer cleanup()
	if _, err := db.ExecContext(ctx, `CREATE TABLE universities (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err == nil || !strings.Contains(err.Error(), "legacy schema is incomplete") {
		t.Fatalf("partial legacy schema was not rejected: %v", err)
	}
}

func TestMigration026UpgradesDatabaseWithOriginal025Applied(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, cleanup := isolatedMigrationSchema(t, ctx, databaseURL)
	defer cleanup()

	if err := applyMigrationsThrough(ctx, db, "024_atomic_source_publication.up.sql"); err != nil {
		t.Fatalf("apply migrations through 024: %v", err)
	}

	suffix := uuid.NewString()
	onlyUniversityID := "migration-only-university-" + suffix
	onlySourceID := "migration-only-source-" + suffix
	onlySnapshotID := "migration-only-snapshot-" + suffix
	competingUniversityID := "migration-competing-university-" + suffix
	inactiveSourceID := "migration-inactive-source-" + suffix
	inactiveSnapshotID := "migration-inactive-snapshot-" + suffix
	activeSourceID := "migration-active-source-" + suffix
	activeSnapshotID := "migration-active-snapshot-" + suffix

	insertMigrationUniversity(t, ctx, db, onlyUniversityID)
	insertPublishedMigrationSource(
		t, ctx, db, onlyUniversityID, onlySourceID, onlySnapshotID, "suspended", false,
	)
	insertMigrationUniversity(t, ctx, db, competingUniversityID)
	insertPublishedMigrationSource(
		t, ctx, db, competingUniversityID, inactiveSourceID, inactiveSnapshotID, "suspended", false,
	)
	insertPublishedMigrationSource(
		t, ctx, db, competingUniversityID, activeSourceID, activeSnapshotID, "active", true,
	)

	if err := applyMigrationsThrough(ctx, db, "025_reconcile_inactive_publications.up.sql"); err != nil {
		t.Fatalf("apply original migration 025: %v", err)
	}
	assertSnapshotPointer(t, ctx, db, onlySourceID, "", "approved")
	assertSnapshotPointer(t, ctx, db, inactiveSourceID, "", "approved")

	// This is the real upgrade path: ApplyMigrations sees 025 in
	// schema_migrations, skips it and executes only the new 026 migration.
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("upgrade database after original migration 025: %v", err)
	}
	assertSnapshotPointer(t, ctx, db, onlySourceID, onlySnapshotID, "published")
	assertSnapshotPointer(t, ctx, db, inactiveSourceID, "", "approved")
	assertSnapshotPointer(t, ctx, db, activeSourceID, activeSnapshotID, "published")

	var queuedSnapshotID string
	if err := db.GetContext(ctx, &queuedSnapshotID, `
		SELECT snapshot_id FROM publication_reconciliation_queue
		WHERE university_id=$1`, competingUniversityID); err != nil {
		t.Fatalf("load queued active publication: %v", err)
	}
	if queuedSnapshotID != activeSnapshotID {
		t.Fatalf("queued snapshot=%q, want active snapshot %q", queuedSnapshotID, activeSnapshotID)
	}
}

func isolatedMigrationSchema(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) (*sqlx.DB, func()) {
	t.Helper()
	base, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "migration_test_" + uuid.NewString()
	schema = "migration_test_" + replaceHyphens(schema[len("migration_test_"):])
	if _, err = base.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_ = db.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = base.ExecContext(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
		_ = base.Close()
	}
	return db, cleanup
}

func replaceHyphens(value string) string {
	result := []byte(value)
	for index := range result {
		if result[index] == '-' {
			result[index] = '_'
		}
	}
	return string(result)
}

func applyMigrationsThrough(ctx context.Context, db *sqlx.DB, lastName string) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`); err != nil {
		return err
	}
	entries, err := fs.Glob(appmigration.Files, "*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		if name > lastName {
			break
		}
		var applied bool
		if err = db.GetContext(ctx, &applied,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)`, name,
		); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, readErr := appmigration.Files.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		tx, beginErr := db.BeginTxx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		if _, execErr := tx.ExecContext(ctx, string(body)); execErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, execErr)
		}
		if _, execErr := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES ($1,NOW())`, name,
		); execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
	}
	return nil
}

func insertMigrationUniversity(t *testing.T, ctx context.Context, db *sqlx.DB, universityID string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities (id, name, full_name, schedule_url)
		VALUES ($1,$1,$1,'https://example.test')`, universityID); err != nil {
		t.Fatal(err)
	}
}

func insertPublishedMigrationSource(
	t *testing.T,
	ctx context.Context,
	db *sqlx.DB,
	universityID, sourceID, snapshotID, lifecycle string,
	enabled bool,
) {
	t.Helper()
	logID := sourceID + "-log"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO data_sources (
			id, university_id, adapter_type, config, update_interval,
			is_enabled, lifecycle_status, last_success_at, last_run_at
		) VALUES ($1,$2,'integration','{}',3600,$3,$4,NOW() - INTERVAL '1 day',NOW() - INTERVAL '1 day')`,
		sourceID, universityID, enabled, lifecycle); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO parse_logs (
			id, data_source_id, started_at, finished_at, status, records_fetched
		) VALUES ($1,$2,NOW() - INTERVAL '1 day',NOW() - INTERVAL '1 day','success',1)`,
		logID, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO parser_snapshots (
			id, data_source_id, parse_log_id, status, publishable,
			group_count, lesson_count, payload, published_at, created_at
		) VALUES ($1,$2,$3,'published',TRUE,1,1,'{}'::jsonb,
		          NOW() - INTERVAL '1 day',NOW() - INTERVAL '1 day')`,
		snapshotID, sourceID, logID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE data_sources SET current_snapshot_id=$2 WHERE id=$1`, sourceID, snapshotID,
	); err != nil {
		t.Fatal(err)
	}
}

func assertSnapshotPointer(
	t *testing.T,
	ctx context.Context,
	db *sqlx.DB,
	sourceID, expectedSnapshotID, expectedStatus string,
) {
	t.Helper()
	var state struct {
		SnapshotID string `db:"snapshot_id"`
		Status     string `db:"status"`
	}
	if err := db.GetContext(ctx, &state, `
		SELECT COALESCE(source.current_snapshot_id, '') AS snapshot_id, snapshot.status
		FROM data_sources source
		JOIN parser_snapshots snapshot ON snapshot.data_source_id=source.id
		WHERE source.id=$1
		ORDER BY snapshot.created_at DESC
		LIMIT 1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if state.SnapshotID != expectedSnapshotID || state.Status != expectedStatus {
		t.Fatalf(
			"source %s state: snapshot=%q status=%q, want snapshot=%q status=%q",
			sourceID, state.SnapshotID, state.Status, expectedSnapshotID, expectedStatus,
		)
	}
}
