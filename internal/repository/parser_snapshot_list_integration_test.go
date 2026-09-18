//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestParserSnapshotListDoesNotReadPayload(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	schema, role := "snapshot_list_"+suffix, "snapshot_reader_"+suffix
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, query := range []string{
			"RESET ROLE", "RESET search_path",
			"DROP SCHEMA IF EXISTS " + schema + " CASCADE",
			"DROP ROLE IF EXISTS " + role,
		} {
			if _, err := db.ExecContext(cleanup, query); err != nil {
				t.Errorf("clean up snapshot list fixture: %v", err)
			}
		}
		_ = db.Close()
	})
	setup := fmt.Sprintf(`
		CREATE SCHEMA %s;
		SET search_path TO %s, pg_catalog;
		CREATE TABLE parser_snapshots (
			id text PRIMARY KEY, data_source_id text NOT NULL, parse_log_id text NOT NULL,
			status text NOT NULL, publishable boolean NOT NULL,
			group_count integer NOT NULL, lesson_count integer NOT NULL,
			anomaly_reasons jsonb NOT NULL, payload jsonb NOT NULL,
			reviewed_by text NOT NULL DEFAULT '', review_note text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL, published_at timestamptz, reviewed_at timestamptz
		);
		INSERT INTO parser_snapshots VALUES
			('new', 'source-a', 'log-new', 'quarantined', true, 1, 20,
			 '[{"code":"lesson_drop","message":"Review required"}]',
			 '{"university_id":"university","groups":[{"id":"group"}]}',
			 'reviewer', 'Checked', '2026-09-18T12:00:00Z', NULL, '2026-09-18T13:00:00Z'),
			('other', 'source-b', 'log-other', 'quarantined', false, 2, 30,
			 '[]', '{}', '', '', '2026-09-17T12:00:00Z', NULL, NULL),
			('old', 'source-a', 'log-old', 'published', true, 3, 40,
			 '[]', '{}', '', '', '2026-09-16T12:00:00Z', '2026-09-16T13:00:00Z', NULL);
		CREATE ROLE %s NOLOGIN;
		GRANT USAGE ON SCHEMA %s TO %s;
		GRANT SELECT (id, data_source_id, parse_log_id, status, publishable,
			group_count, lesson_count, anomaly_reasons, reviewed_by, review_note,
			created_at, published_at, reviewed_at) ON parser_snapshots TO %s;
		SET ROLE %s;
	`, schema, schema, role, schema, role, role, role)
	if _, err := db.ExecContext(ctx, setup); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewParserSnapshotRepository(db)
	for _, tc := range []struct {
		name, source, status string
		limit                int
		want                 string
	}{
		{"all", "", "", 100, "new,other,old"},
		{"source", "source-a", "", 100, "new,old"},
		{"status", "", "published", 100, "old"},
		{"combined", "source-a", "quarantined", 100, "new"},
		{"limit", "", "", 1, "new"},
		{"minimum limit", "", "", 0, "new"},
		{"empty", "missing", "", 100, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := repo.List(ctx, tc.source, tc.status, tc.limit)
			if err != nil {
				t.Fatalf("list must only read metadata columns: %v", err)
			}
			ids := make([]string, 0, len(items))
			for _, item := range items {
				ids = append(ids, item.ID)
				if item.Payload.UniversityID != "" || len(item.Payload.Groups) != 0 {
					t.Fatal("list unexpectedly loaded a schedule payload")
				}
				if item.ID == "new" && (item.LessonCount != 20 || len(item.AnomalyReasons) != 1 ||
					item.AnomalyReasons[0].Code != "lesson_drop" || item.ReviewNote != "Checked" ||
					item.ReviewedBy != "reviewer" || item.ReviewedAt == nil) {
					t.Fatalf("snapshot metadata lost: %+v", item)
				}
			}
			if got := strings.Join(ids, ","); got != tc.want {
				t.Fatalf("ids = %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := db.ExecContext(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.Get(ctx, "new")
	if err != nil || snapshot == nil || snapshot.Payload.UniversityID != "university" ||
		len(snapshot.Payload.Groups) != 1 {
		t.Fatalf("individual snapshot must still load its full payload: snapshot=%+v err=%v", snapshot, err)
	}
}
