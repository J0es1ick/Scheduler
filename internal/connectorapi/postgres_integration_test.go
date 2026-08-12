//go:build integration

package connectorapi

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestSignedSnapshotIntakeAndStaging(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()
	universityID := "connector-integration-" + suffix
	connectorID := "connector-" + suffix
	sourceID := "source-" + suffix
	keyID := "key-" + suffix
	publicEncoded, privateEncoded, err := connector.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := connector.DecodePublicKey(publicEncoded)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewConnectorRepository(db)
	_, err = repo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: sourceID, UniversityID: universityID,
		UniversityName: "Connector Integration", Timezone: "Europe/Moscow", Locale: "ru-RU",
		DisplayName: "Integration connector", KeyID: keyID, PublicKey: publicKey,
		CreatedBy: "integration", QualityPolicy: domain.DefaultSourceQualityPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, cleanup := range []struct {
			query string
			arg   string
		}{
			{`DELETE FROM connector_request_nonces WHERE connector_id=$1`, connectorID},
			{`DELETE FROM connector_ingestion_runs WHERE connector_id=$1`, connectorID},
			{`DELETE FROM connector_clients WHERE id=$1`, connectorID},
			{`DELETE FROM universities WHERE id=$1`, universityID},
		} {
			if _, cleanupErr := db.ExecContext(cleanupCtx, cleanup.query, cleanup.arg); cleanupErr != nil {
				t.Errorf("cleanup connector integration fixture %q: %v", cleanup.arg, cleanupErr)
			}
		}
	})
	if err = repo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusTesting); err != nil {
		t.Fatal(err)
	}

	groupRepo := repository.NewGroupRepository(db)
	parser := service.NewParserService(
		repository.NewDataSourceRepository(db), repository.NewParseLogRepository(db), groupRepo,
		service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), groupRepo),
		repository.NewParserSnapshotRepository(db), repository.NewNotificationRepository(db),
		repository.NewParserDiagnosticRepository(db),
	)
	ingestion := NewService(repo, parser)
	server := httptest.NewServer(NewServer(repo).Handler())
	defer server.Close()
	client, err := connector.NewClient(server.URL, connectorID, keyID, privateEncoded)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := connector.Snapshot{
		SchemaVersion: connector.SchemaVersion, SnapshotID: "snapshot-" + suffix, GeneratedAt: time.Now().UTC(),
		Institution: connector.Institution{ExternalID: universityID, Name: "Connector Integration", Timezone: "Europe/Moscow"},
		Term:        connector.Term{ExternalID: "2026-autumn", Name: "Autumn", StartsOn: "2026-09-01", EndsOn: "2027-01-31"},
		Groups: []connector.Group{{ExternalID: "group-1", Name: "1/1", Lessons: []connector.Lesson{{
			ExternalID: "lesson-1", Subject: "Mathematics", Type: "lecture",
			Schedule: connector.Schedule{DayOfWeek: 1, StartsAt: "09:00", EndsAt: "10:30", Recurrence: connector.Recurrence{Kind: connector.RecurrenceEvery}},
		}}}},
	}
	submission, err := client.Submit(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != domain.IngestionStatusReceived {
		t.Fatalf("submission status=%s", submission.Status)
	}
	processed, err := ingestion.ProcessNext(ctx)
	if err != nil || !processed {
		t.Fatalf("process snapshot: processed=%t err=%v", processed, err)
	}
	status, err := client.Run(ctx, submission.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != domain.IngestionStatusStaged || status.GroupCount != 1 || status.LessonCount != 1 {
		t.Fatalf("unexpected run: %+v", status)
	}
}
