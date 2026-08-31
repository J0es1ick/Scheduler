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
	"github.com/jackc/pgx/v5"
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

func TestRuntimeDatabasePrivileges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	botRole := "bot_test_" + replaceHyphens(uuid.NewString())
	adminRole := "admin_test_" + replaceHyphens(uuid.NewString())
	for _, role := range []string{botRole, adminRole} {
		if _, err = db.ExecContext(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.ExecContext(context.Background(), "DROP OWNED BY "+pgx.Identifier{role}.Sanitize())
			db.ExecContext(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
		})
	}
	if _, err = db.ExecContext(ctx, "GRANT UPDATE (admin_role) ON users TO "+pgx.Identifier{botRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err = ApplyRuntimeGrants(ctx, db, botRole, adminRole); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		role, table, privilege string
		want                   bool
	}{
		{botRole, "admin_sessions", "INSERT", false}, {botRole, "schema_migrations", "UPDATE", false},
		{botRole, "lesson_overrides", "INSERT", false}, {botRole, "subscriptions", "INSERT", true},
		{botRole, "notification_deliveries", "UPDATE", true}, {adminRole, "worker_status", "UPDATE", false},
		{adminRole, "schema_migrations", "DELETE", false}, {adminRole, "admin_sessions", "INSERT", true},
	} {
		var allowed bool
		if err = db.GetContext(ctx, &allowed, `SELECT has_table_privilege($1,$2,$3)`, test.role, test.table, test.privilege); err != nil {
			t.Fatal(err)
		}
		if allowed != test.want {
			t.Errorf("%s %s %s: %t", test.role, test.table, test.privilege, allowed)
		}
	}
	var csrfReadable bool
	if err = db.GetContext(ctx, &csrfReadable, `SELECT has_column_privilege($1,'admin_sessions','csrf_token','SELECT')`, botRole); err != nil || csrfReadable {
		t.Fatalf("bot can read admin CSRF secrets: %t %v", csrfReadable, err)
	}
	var rolesWritable bool
	if err = db.GetContext(ctx, &rolesWritable, `SELECT has_column_privilege($1,'users','admin_role','UPDATE') OR has_column_privilege($1,'users','is_admin','INSERT')`, botRole); err != nil || rolesWritable {
		t.Fatalf("bot can assign administrative roles: %t %v", rolesWritable, err)
	}
	var searchViewWritable bool
	if err = db.GetContext(ctx, &searchViewWritable, `SELECT has_column_privilege($1,'users','search_schedule_view_format','UPDATE')`, botRole); err != nil || !searchViewWritable {
		t.Fatalf("bot cannot update search schedule format: %t %v", searchViewWritable, err)
	}
}

func TestReconcileHistoricalSubscriptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "037_notification_delivery_leases.up.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities (id,name,full_name,schedule_url) VALUES ('u','U','University','https://example.test');
		INSERT INTO groups (id,university_id,name,is_active) VALUES ('g','u','G',FALSE), ('g2','u','G2',TRUE);
		INSERT INTO users (id,default_group_id) VALUES ('legacy','g'), ('existing','g2');
		INSERT INTO subscriptions (id,user_id,object_id,object_type,schedule_view_format,subgroup)
		VALUES ('orphan','legacy','deleted','group','visual',0), ('preserved','existing','g2','group','compact',17);
	`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err := db.GetContext(ctx, &valid, `SELECT
		EXISTS (SELECT 1 FROM subscriptions WHERE user_id='legacy' AND group_id='g')
		AND EXISTS (SELECT 1 FROM subscriptions WHERE id='preserved' AND subgroup=17 AND schedule_view_format='compact')
		AND NOT EXISTS (SELECT 1 FROM subscriptions WHERE id='orphan')`); err != nil || !valid {
		t.Fatalf("reconciliation lost preferences: valid=%t err=%v", valid, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO subscriptions (id,user_id,object_id,object_type) VALUES ('bad','legacy','missing','group')`); err == nil {
		t.Fatal("orphan group subscription accepted")
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET default_group_id='g2' WHERE id='legacy'`); err == nil {
		t.Fatal("default without subscription accepted")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM groups WHERE id='g'`); err != nil {
		t.Fatalf("group deletion did not clear references: %v", err)
	}
	if err := CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMigrationsIsReadOnlyAndRejectsDrift(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, cleanup := isolatedMigrationSchema(t, ctx, databaseURL)
	defer cleanup()

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := VerifyMigrations(ctx, db); err != nil {
		t.Fatalf("verify current migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE schema_migrations SET checksum='drift' WHERE name=(SELECT MIN(name) FROM schema_migrations)`); err != nil {
		t.Fatalf("tamper migration metadata: %v", err)
	}
	if err := VerifyMigrations(ctx, db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("verify drift error = %v, want checksum mismatch", err)
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
