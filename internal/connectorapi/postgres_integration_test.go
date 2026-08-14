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
	oldSourceID := "old-source-" + suffix
	oldParseLogID := "old-parse-" + suffix
	oldSnapshotID := "old-snapshot-" + suffix
	oldSemesterID := "old-semester-" + suffix
	oldGroupID := "old-group-" + suffix
	periodStart := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC)
	dataSources := repository.NewDataSourceRepository(db)
	if _, err = dataSources.CreateDataSource(
		ctx, oldSourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}
	parseLogs := repository.NewParseLogRepository(db)
	if _, err = parseLogs.CreateParseLog(ctx, oldParseLogID, oldSourceID, "running", 0, ""); err != nil {
		t.Fatal(err)
	}
	snapshots := repository.NewParserSnapshotRepository(db)
	if err = snapshots.Create(ctx, &domain.ParserSnapshot{
		ID: oldSnapshotID, DataSourceID: oldSourceID, ParseLogID: oldParseLogID,
		Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1, LessonCount: 1,
		Payload: domain.ScheduleSnapshot{
			UniversityID: universityID, SemesterID: oldSemesterID,
			StartDate: periodStart, EndDate: periodEnd,
			Groups: []domain.SnapshotGroup{{
				ID: oldGroupID, UniversityID: universityID, Name: "1/1",
				Lessons: []domain.Lesson{{
					ID: "old-lesson-" + suffix, UniversityID: universityID,
					SemesterID: oldSemesterID, DayOfWeek: 2,
					TimeStart: "09:00", TimeEnd: "10:30", WeekType: domain.WeekTypeEvery,
					Subject: "Old schedule", Type: domain.LessonTypeLecture,
					GroupID: oldGroupID, ValidFrom: &periodStart, ValidTo: &periodEnd,
				}},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshots.Publish(ctx, oldSnapshotID, "integration", "initial live schedule"); err != nil {
		t.Fatal(err)
	}
	manualOverrideID := "manual-override-" + suffix
	if _, err = db.ExecContext(ctx, `
		INSERT INTO lesson_overrides (
			id, university_id, semester_id, day_of_week, time_start, time_end,
			week_type, subject, type, group_id, valid_from, valid_to, created_by
		) VALUES ($1,$2,$3,3,'12:00','13:30','every',$4,'practice',$5,$6,$7,$8)`,
		manualOverrideID, universityID, oldSemesterID, "Manual consultation",
		oldGroupID, periodStart, periodEnd, "integration",
	); err != nil {
		t.Fatalf("create manual override: %v", err)
	}
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
		Institution: connector.Institution{ExternalID: universityID, Name: "Published Connector Integration", Timezone: "Europe/Moscow"},
		Term:        connector.Term{ExternalID: "2026-autumn", Name: "Autumn", StartsOn: "2026-08-31", EndsOn: "2027-01-31"},
		Groups: []connector.Group{{ExternalID: "group-1", Name: "1/1", Lessons: []connector.Lesson{{
			ExternalID: "lesson-1", Subject: "Mathematics", Type: "lecture",
			Schedule: connector.Schedule{DayOfWeek: 2, StartsAt: "09:00", EndsAt: "10:30", Recurrence: connector.Recurrence{Kind: connector.RecurrenceOdd}},
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
	candidate, err := snapshots.Get(ctx, status.ParserSnapshotID)
	if err != nil || candidate == nil {
		t.Fatalf("load staged snapshot: candidate=%+v err=%v", candidate, err)
	}
	var nameBeforeActivation string
	if err = db.GetContext(ctx, &nameBeforeActivation,
		`SELECT name FROM universities WHERE id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if nameBeforeActivation != "Connector Integration" {
		t.Fatalf("test snapshot changed live metadata before approval: %q", nameBeforeActivation)
	}
	reviewed, err := parser.PublishSnapshot(ctx, candidate.ID, "integration", "approved")
	if err != nil {
		t.Fatalf("approve connector snapshot: %v", err)
	}
	if reviewed.Status != domain.SnapshotStatusApproved {
		t.Fatalf("reviewed snapshot status=%s, want approved", reviewed.Status)
	}
	var lessonsBeforeActivation int
	if err = db.GetContext(ctx, &lessonsBeforeActivation,
		`SELECT COUNT(*)::int FROM lessons WHERE university_id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if lessonsBeforeActivation != 1 {
		t.Fatalf("approved snapshot changed live schedule: lessons=%d, want old lesson", lessonsBeforeActivation)
	}
	oldPublicationEntered := make(chan struct{})
	releaseOldPublication := make(chan struct{})
	oldPublicationDone := make(chan error, 1)
	go func() {
		_, publishErr := snapshots.PublishWithHook(
			ctx, oldSnapshotID, "integration", "concurrent old publication",
			func(hookCtx context.Context, _ *repository.SnapshotPublication) error {
				close(oldPublicationEntered)
				select {
				case <-releaseOldPublication:
					return nil
				case <-hookCtx.Done():
					return hookCtx.Err()
				}
			},
		)
		oldPublicationDone <- publishErr
	}()
	select {
	case <-oldPublicationEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent old publication did not acquire the university lock")
	}
	type activationResult struct {
		snapshot *domain.ParserSnapshot
		err      error
	}
	activationDone := make(chan activationResult, 1)
	go func() {
		activated, activateErr := parser.ActivateConnector(
			ctx, connectorID, candidate.ID, "integration", "activate approved source",
		)
		activationDone <- activationResult{snapshot: activated, err: activateErr}
	}()
	select {
	case result := <-activationDone:
		t.Fatalf("connector activation bypassed the publication lock: %v", result.err)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseOldPublication)
	if publishErr := <-oldPublicationDone; publishErr != nil {
		t.Fatalf("concurrent old publication failed before source switch: %v", publishErr)
	}
	result := <-activationDone
	if result.err != nil {
		t.Fatalf("activate connector snapshot: %v", result.err)
	}
	activated := result.snapshot
	var nameAfterActivation string
	if err = db.GetContext(ctx, &nameAfterActivation,
		`SELECT name FROM universities WHERE id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if nameAfterActivation != "Published Connector Integration" {
		t.Fatalf("published metadata name=%q", nameAfterActivation)
	}
	var oldLifecycle, newLifecycle string
	if err = db.GetContext(ctx, &oldLifecycle,
		`SELECT lifecycle_status FROM data_sources WHERE id=$1`, oldSourceID); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &newLifecycle,
		`SELECT lifecycle_status FROM data_sources WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if oldLifecycle != domain.ConnectorStatusSuspended || newLifecycle != domain.ConnectorStatusActive {
		t.Fatalf("source switch was not atomic: old=%s new=%s", oldLifecycle, newLifecycle)
	}
	var manualOverrides int
	if err = db.GetContext(ctx, &manualOverrides,
		`SELECT COUNT(*)::int FROM effective_lessons WHERE id=$1 AND origin='manual'`,
		manualOverrideID); err != nil {
		t.Fatal(err)
	}
	if manualOverrides != 1 {
		t.Fatalf("source switch removed manual override: count=%d", manualOverrides)
	}
	if _, err = snapshots.Publish(ctx, oldSnapshotID, "integration", "stale retry"); err == nil {
		t.Fatal("suspended source was allowed to overwrite the active schedule")
	}
	schedule := service.NewScheduleService(
		repository.NewLessonRepository(db), repository.NewSemesterRepository(db), groupRepo,
	)
	groupID := activated.Payload.Groups[0].ID
	oddWeek, err := schedule.GetScheduleForGroup(ctx, groupID, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(oddWeek) != 1 {
		t.Fatalf("odd week schedule: lessons=%d err=%v", len(oddWeek), err)
	}
	evenWeek, err := schedule.GetScheduleForGroup(ctx, groupID, time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	if err != nil || len(evenWeek) != 0 {
		t.Fatalf("even week schedule: lessons=%d err=%v", len(evenWeek), err)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM lessons WHERE university_id=$1`, universityID); err != nil {
		t.Fatalf("simulate damaged live schedule: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE data_sources
		SET last_error='temporary upstream failure', consecutive_failures=3,
		    next_retry_at=NOW() + INTERVAL '15 minutes'
		WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("put active source into backoff before reconciliation: %v", err)
	}
	var sourceHistoryBefore struct {
		LastSuccessAt      *time.Time `db:"last_success_at"`
		LastRunAt          *time.Time `db:"last_run_at"`
		LastError          string     `db:"last_error"`
		ConsecutiveFailure int        `db:"consecutive_failures"`
		NextRetryAt        *time.Time `db:"next_retry_at"`
		UpdatedAt          time.Time  `db:"updated_at"`
		PublishedAt        time.Time  `db:"published_at"`
	}
	if err = db.GetContext(ctx, &sourceHistoryBefore, `
		SELECT source.last_success_at, source.last_run_at, source.last_error,
		       source.consecutive_failures, source.next_retry_at, source.updated_at,
		       snapshot.published_at
		FROM data_sources source
		JOIN parser_snapshots snapshot ON snapshot.id=source.current_snapshot_id
		WHERE source.id=$1`, sourceID); err != nil {
		t.Fatalf("load source history before reconciliation: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (university_id, snapshot_id, reason)
		VALUES ($1,$2,'integration recovery test')
		ON CONFLICT (university_id) DO UPDATE SET
			snapshot_id=EXCLUDED.snapshot_id, reason=EXCLUDED.reason,
			attempts=0, claim_token='', claimed_at=NULL, last_error=''`,
		universityID, candidate.ID,
	); err != nil {
		t.Fatalf("queue publication reconciliation: %v", err)
	}
	type reconciliationResult struct {
		count int
		err   error
	}
	reconciliations := make(chan reconciliationResult, 2)
	for range 2 {
		go func() {
			count, reconcileErr := snapshots.ReconcilePendingPublications(ctx)
			reconciliations <- reconciliationResult{count: count, err: reconcileErr}
		}()
	}
	totalReconciled := 0
	for range 2 {
		result := <-reconciliations
		if result.err != nil {
			t.Fatalf("reconcile trusted publication concurrently: %v", result.err)
		}
		totalReconciled += result.count
	}
	if totalReconciled != 1 {
		t.Fatalf("concurrent reconciliation count=%d, want exactly one publication", totalReconciled)
	}
	var restoredLessons, preservedOverrides int
	if err = db.GetContext(ctx, &restoredLessons,
		`SELECT COUNT(*)::int FROM lessons WHERE university_id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &preservedOverrides,
		`SELECT COUNT(*)::int FROM effective_lessons WHERE id=$1 AND origin='manual'`,
		manualOverrideID); err != nil {
		t.Fatal(err)
	}
	if restoredLessons != 1 || preservedOverrides != 1 {
		t.Fatalf(
			"reconciliation result: parsed lessons=%d manual overrides=%d",
			restoredLessons, preservedOverrides,
		)
	}
	var sourceHistoryAfter struct {
		LastSuccessAt      *time.Time `db:"last_success_at"`
		LastRunAt          *time.Time `db:"last_run_at"`
		LastError          string     `db:"last_error"`
		ConsecutiveFailure int        `db:"consecutive_failures"`
		NextRetryAt        *time.Time `db:"next_retry_at"`
		UpdatedAt          time.Time  `db:"updated_at"`
		PublishedAt        time.Time  `db:"published_at"`
	}
	if err = db.GetContext(ctx, &sourceHistoryAfter, `
		SELECT source.last_success_at, source.last_run_at, source.last_error,
		       source.consecutive_failures, source.next_retry_at, source.updated_at,
		       snapshot.published_at
		FROM data_sources source
		JOIN parser_snapshots snapshot ON snapshot.id=source.current_snapshot_id
		WHERE source.id=$1`, sourceID); err != nil {
		t.Fatalf("load source history after reconciliation: %v", err)
	}
	if !optionalTimesEqual(sourceHistoryBefore.LastSuccessAt, sourceHistoryAfter.LastSuccessAt) ||
		!optionalTimesEqual(sourceHistoryBefore.LastRunAt, sourceHistoryAfter.LastRunAt) ||
		sourceHistoryBefore.LastError != sourceHistoryAfter.LastError ||
		sourceHistoryBefore.ConsecutiveFailure != sourceHistoryAfter.ConsecutiveFailure ||
		!optionalTimesEqual(sourceHistoryBefore.NextRetryAt, sourceHistoryAfter.NextRetryAt) ||
		!sourceHistoryBefore.UpdatedAt.Equal(sourceHistoryAfter.UpdatedAt) ||
		!sourceHistoryBefore.PublishedAt.Equal(sourceHistoryAfter.PublishedAt) {
		t.Fatalf(
			"reconciliation changed parser history: before=%+v after=%+v",
			sourceHistoryBefore, sourceHistoryAfter,
		)
	}
	var remainingReconciliations int
	if err = db.GetContext(ctx, &remainingReconciliations,
		`SELECT COUNT(*)::int FROM publication_reconciliation_queue WHERE university_id=$1`,
		universityID); err != nil {
		t.Fatal(err)
	}
	if remainingReconciliations != 0 {
		t.Fatalf("completed reconciliation was retained: count=%d", remainingReconciliations)
	}
}

func optionalTimesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func TestConnectorActivationRollsBackOnPublicationFailure(t *testing.T) {
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
	universityID := "connector-rollback-" + suffix
	connectorID := "connector-rollback-client-" + suffix
	newSourceID := "connector-rollback-new-" + suffix
	oldSourceID := "connector-rollback-old-" + suffix
	repo := repository.NewConnectorRepository(db)
	if _, err = repo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID: connectorID, SourceID: newSourceID, UniversityID: universityID,
		UniversityName: "Atomic rollback", Timezone: "Europe/Moscow", Locale: "ru-RU",
		DisplayName: "Broken candidate", KeyID: "rollback-key-" + suffix,
		PublicKey: []byte{}, CreatedBy: "integration",
		QualityPolicy: domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})
	if err = repo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusTesting); err != nil {
		t.Fatal(err)
	}
	if err = repo.UpdateStatus(ctx, connectorID, domain.ConnectorStatusPendingReview); err != nil {
		t.Fatal(err)
	}

	dataSources := repository.NewDataSourceRepository(db)
	if _, err = dataSources.CreateDataSource(ctx, oldSourceID, universityID, "integration", "{}", 3600); err != nil {
		t.Fatal(err)
	}
	parseLogs := repository.NewParseLogRepository(db)
	snapshots := repository.NewParserSnapshotRepository(db)
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC)
	oldLogID := "rollback-old-log-" + suffix
	if _, err = parseLogs.CreateParseLog(ctx, oldLogID, oldSourceID, "running", 0, ""); err != nil {
		t.Fatal(err)
	}
	oldGroupID := "rollback-old-group-" + suffix
	oldSnapshotID := "rollback-old-snapshot-" + suffix
	if err = snapshots.Create(ctx, &domain.ParserSnapshot{
		ID: oldSnapshotID, DataSourceID: oldSourceID, ParseLogID: oldLogID,
		Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1, LessonCount: 1,
		Payload: domain.ScheduleSnapshot{
			UniversityID: universityID, SemesterID: "rollback-old-term-" + suffix,
			StartDate: start, EndDate: end,
			Groups: []domain.SnapshotGroup{{
				ID: oldGroupID, UniversityID: universityID, Name: "ROLLBACK-1",
				Lessons: []domain.Lesson{{
					ID: "rollback-old-lesson-" + suffix, UniversityID: universityID,
					DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:30",
					WeekType: domain.WeekTypeEvery, Subject: "Stable live lesson",
					Type: domain.LessonTypeLecture, GroupID: oldGroupID,
					ValidFrom: &start, ValidTo: &end,
				}},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshots.Publish(ctx, oldSnapshotID, "integration", "stable baseline"); err != nil {
		t.Fatal(err)
	}

	newLogID := "rollback-new-log-" + suffix
	if _, err = parseLogs.CreateParseLog(ctx, newLogID, newSourceID, "running", 0, ""); err != nil {
		t.Fatal(err)
	}
	duplicateLessonID := "rollback-duplicate-lesson-" + suffix
	newSnapshotID := "rollback-new-snapshot-" + suffix
	newGroupID := "rollback-new-group-" + suffix
	brokenLessons := []domain.Lesson{
		{
			ID: duplicateLessonID, UniversityID: universityID, DayOfWeek: 1,
			TimeStart: "10:00", TimeEnd: "11:30", WeekType: domain.WeekTypeEvery,
			Subject: "First duplicate", Type: domain.LessonTypeLecture,
			GroupID: newGroupID, ValidFrom: &start, ValidTo: &end,
		},
		{
			ID: duplicateLessonID, UniversityID: universityID, DayOfWeek: 2,
			TimeStart: "10:00", TimeEnd: "11:30", WeekType: domain.WeekTypeEvery,
			Subject: "Second duplicate", Type: domain.LessonTypeLecture,
			GroupID: newGroupID, ValidFrom: &start, ValidTo: &end,
		},
	}
	if err = snapshots.Create(ctx, &domain.ParserSnapshot{
		ID: newSnapshotID, DataSourceID: newSourceID, ParseLogID: newLogID,
		Status: domain.SnapshotStatusApproved, Publishable: true, GroupCount: 1, LessonCount: 2,
		Payload: domain.ScheduleSnapshot{
			UniversityID: universityID, SemesterID: "rollback-new-term-" + suffix,
			StartDate: start, EndDate: end,
			Groups: []domain.SnapshotGroup{{
				ID: newGroupID, UniversityID: universityID, Name: "ROLLBACK-1", Lessons: brokenLessons,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshots.ActivateConnectorWithSnapshot(
		ctx, connectorID, newSnapshotID, "integration", "must roll back", nil,
	); err == nil {
		t.Fatal("broken publication unexpectedly activated the connector")
	}

	var oldLifecycle, newLifecycle, connectorStatus string
	if err = db.GetContext(ctx, &oldLifecycle,
		`SELECT lifecycle_status FROM data_sources WHERE id=$1`, oldSourceID); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &newLifecycle,
		`SELECT lifecycle_status FROM data_sources WHERE id=$1`, newSourceID); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &connectorStatus,
		`SELECT status FROM connector_clients WHERE id=$1`, connectorID); err != nil {
		t.Fatal(err)
	}
	if oldLifecycle != domain.ConnectorStatusActive ||
		newLifecycle != domain.ConnectorStatusPendingReview ||
		connectorStatus != domain.ConnectorStatusPendingReview {
		t.Fatalf(
			"failed activation leaked lifecycle changes: old=%s new=%s connector=%s",
			oldLifecycle, newLifecycle, connectorStatus,
		)
	}
	var stableLessons int
	if err = db.GetContext(ctx, &stableLessons, `
		SELECT COUNT(*)::int FROM lessons
		WHERE university_id=$1 AND subject='Stable live lesson'`, universityID); err != nil {
		t.Fatal(err)
	}
	if stableLessons != 1 {
		t.Fatalf("failed activation damaged live schedule: stable lessons=%d", stableLessons)
	}
}
