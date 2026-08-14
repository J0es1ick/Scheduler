//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestConnectorLeaseRejectsLateWorkerCompletion(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()
	connectorID := "lease-connector-" + suffix
	sourceID := "lease-source-" + suffix
	universityID := "lease-university-" + suffix
	repo := repository.NewConnectorRepository(db)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_ingestion_runs WHERE connector_id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM data_sources WHERE id=$1`, sourceID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})
	if _, err = repo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: sourceID, UniversityID: universityID,
		UniversityName: "Lease test", DisplayName: "Lease test", KeyID: "lease-key-" + suffix,
		PublicKey: []byte("integration-public-key"), CreatedBy: "integration",
		QualityPolicy: domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = repo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusTesting); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.Enqueue(ctx, connectorID, "snapshot-"+suffix, "1.0",
		"idempotency-"+suffix, "digest", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimNext(ctx)
	if err != nil || first == nil || first.ClaimToken == "" || first.LeaseExpiresAt == nil {
		t.Fatalf("first claim: run=%+v err=%v", first, err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE connector_ingestion_runs SET lease_expires_at=NOW()-INTERVAL '1 second'
		WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ClaimNext(ctx)
	if err != nil || second == nil || second.ID != first.ID || second.ClaimToken == first.ClaimToken {
		t.Fatalf("reclaimed run: first=%+v second=%+v err=%v", first, second, err)
	}
	if err = repo.Complete(ctx, first.ID, first.ClaimToken, domain.IngestionStatusStaged, "", 0, 0); !errors.Is(err, repository.ErrConnectorClaimLost) {
		t.Fatalf("late completion was accepted: %v", err)
	}
	if err = repo.Fail(ctx, first.ID, first.ClaimToken, errors.New("late worker"), false); !errors.Is(err, repository.ErrConnectorClaimLost) {
		t.Fatalf("late failure was accepted: %v", err)
	}
	if err = repo.RenewClaim(ctx, second.ID, second.ClaimToken); err != nil {
		t.Fatalf("renew active claim: %v", err)
	}
	if err = repo.Complete(ctx, second.ID, second.ClaimToken, domain.IngestionStatusStaged, "", 0, 0); err != nil {
		t.Fatalf("complete active claim: %v", err)
	}
}

func TestExpiredConnectorLeaseCannotPublishSchedule(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()
	connectorID := "fence-connector-" + suffix
	sourceID := "fence-source-" + suffix
	universityID := "fence-university-" + suffix
	groupID := "fence-group-" + suffix
	runRepo := repository.NewConnectorRepository(db)
	if _, err = runRepo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: sourceID, UniversityID: universityID,
		UniversityName: "Fencing test", DisplayName: "Fencing test",
		KeyID: "fence-key-" + suffix, PublicKey: []byte("integration-public-key"),
		CreatedBy: "integration", QualityPolicy: domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_ingestion_runs WHERE connector_id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})
	if err = runRepo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusActive); err != nil {
		t.Fatal(err)
	}
	if _, _, err = runRepo.Enqueue(
		ctx, connectorID, "fence-snapshot-"+suffix, "1.0",
		"fence-idempotency-"+suffix, "digest", []byte(`{}`),
	); err != nil {
		t.Fatal(err)
	}
	run, err := runRepo.ClaimNext(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim connector run: run=%+v err=%v", run, err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE connector_ingestion_runs SET lease_expires_at=NOW()-INTERVAL '1 second'
		WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}

	groupRepo := repository.NewGroupRepository(db)
	parser := service.NewParserService(
		repository.NewDataSourceRepository(db), repository.NewParseLogRepository(db), groupRepo,
		service.NewScheduleService(
			repository.NewLessonRepository(db), repository.NewSemesterRepository(db), groupRepo,
		),
		repository.NewParserSnapshotRepository(db), repository.NewNotificationRepository(db),
		repository.NewParserDiagnosticRepository(db),
	)
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	_, err = parser.IngestClaimedExternalSnapshot(ctx, sourceID, domain.ScheduleSnapshot{
		UniversityID: universityID,
		SemesterID:   "fence-semester-" + suffix,
		StartDate:    start,
		EndDate:      end,
		Groups: []domain.SnapshotGroup{{
			ID: groupID, UniversityID: universityID, Name: "FENCE",
			Lessons: []domain.Lesson{{
				ID: "fence-lesson-" + suffix, ExternalID: "lesson-1",
				UniversityID: universityID, DayOfWeek: 1,
				TimeStart: "08:00", TimeEnd: "09:35", WeekType: domain.WeekTypeEvery,
				Subject: "Must not publish", Type: domain.LessonTypeLecture,
				GroupID: groupID, ValidFrom: &start, ValidTo: &end,
			}},
		}},
	}, run.ID, run.ClaimToken)
	if !errors.Is(err, repository.ErrConnectorClaimLost) {
		t.Fatalf("expired lease publication error=%v, want ErrConnectorClaimLost", err)
	}
	var liveLessons int
	if err = db.GetContext(ctx, &liveLessons,
		`SELECT COUNT(*)::int FROM lessons WHERE university_id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if liveLessons != 0 {
		t.Fatalf("expired connector lease published %d lessons", liveLessons)
	}
}
