//go:build integration

package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestSourceLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := uuid.NewString()
	universityID := "source-lifecycle-university-" + suffix
	sourceID := "source-lifecycle-source-" + suffix
	parseLogID := "source-lifecycle-log-" + suffix
	snapshotID := "source-lifecycle-snapshot-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})

	if _, err = db.ExecContext(ctx, `
		INSERT INTO universities (id, name, full_name, schedule_url, is_active)
		VALUES ($1, 'Lifecycle test', 'Lifecycle test', 'https://example.test', TRUE)`,
		universityID,
	); err != nil {
		t.Fatalf("create university: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval)
		VALUES ($1, $2, 'integration', '{}', 3600)`, sourceID, universityID,
	); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO parse_logs (
			id, data_source_id, started_at, finished_at, status, records_fetched, error_message
		) VALUES ($1, $2, NOW(), NOW(), 'success', 0, '')`, parseLogID, sourceID,
	); err != nil {
		t.Fatalf("create parse log: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO parser_snapshots (
			id, data_source_id, parse_log_id, status, publishable,
			group_count, lesson_count, anomaly_reasons, payload
		) VALUES ($1, $2, $3, 'published', TRUE, 0, 0, '[]', '{}')`,
		snapshotID, sourceID, parseLogID,
	); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if _, err = db.ExecContext(ctx,
		`UPDATE data_sources SET current_snapshot_id=$1 WHERE id=$2`, snapshotID, sourceID,
	); err != nil {
		t.Fatalf("select current snapshot: %v", err)
	}

	store := NewStore(db)
	disabled := false
	if err = store.UpdateSourceSettings(ctx, sourceID, nil, &disabled); err != nil {
		t.Fatalf("disable source: %v", err)
	}
	enabled, err := store.SourceEnabled(ctx, sourceID)
	if err != nil || enabled {
		t.Fatalf("disabled source state: enabled=%v err=%v", enabled, err)
	}

	enabled = true
	if err = store.UpdateSourceSettings(ctx, sourceID, nil, &enabled); err != nil {
		t.Fatalf("enable source: %v", err)
	}
	if enabled, err = store.SourceEnabled(ctx, sourceID); err != nil || !enabled {
		t.Fatalf("enabled source state: enabled=%v err=%v", enabled, err)
	}

	if err = store.DeleteSource(ctx, sourceID); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	activeSources, err := store.Sources(ctx, false)
	if err != nil {
		t.Fatalf("list active sources: %v", err)
	}
	if containsSource(activeSources, sourceID) {
		t.Fatal("archived source is present in the active source list")
	}
	allSources, err := store.Sources(ctx, true)
	if err != nil {
		t.Fatalf("list all sources: %v", err)
	}
	if !containsSource(allSources, sourceID) {
		t.Fatal("archived source is missing from the archive-capable source list")
	}
	for table, query := range map[string]string{
		"data_sources":     `SELECT COUNT(*) FROM data_sources WHERE id=$1 AND lifecycle_status='archived'`,
		"parse_logs":       `SELECT COUNT(*) FROM parse_logs WHERE data_source_id=$1`,
		"parser_snapshots": `SELECT COUNT(*) FROM parser_snapshots WHERE data_source_id=$1`,
	} {
		var count int
		if err = db.GetContext(ctx, &count, query, sourceID); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("%s contains %d retained rows after source archival", table, count)
		}
	}
	status, err := store.RestoreSource(ctx, sourceID)
	if err != nil {
		t.Fatalf("restore source: %v", err)
	}
	if status != "active" {
		t.Fatalf("restored lifecycle=%q, want active", status)
	}
	var restored struct {
		Lifecycle string `db:"lifecycle_status"`
		Enabled   bool   `db:"is_enabled"`
	}
	if err = db.GetContext(ctx, &restored,
		`SELECT lifecycle_status, is_enabled FROM data_sources WHERE id=$1`, sourceID,
	); err != nil {
		t.Fatalf("load restored source: %v", err)
	}
	if restored.Lifecycle != "active" || restored.Enabled {
		t.Fatalf("unexpected restored source: %+v", restored)
	}
}

func containsSource(items []SourceView, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
