//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestPostgresRepositoryFlow(t *testing.T) {
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
	universityID := "integration-university-" + suffix
	groupID := "integration-group-" + suffix
	userID := "integration-user-" + suffix
	subscriptionID := "integration-subscription-" + suffix
	workerName := "integration-worker-" + suffix
	dataSourceID := "integration-source-" + suffix
	parseLogID := "integration-parse-log-" + suffix
	snapshotID := "integration-snapshot-" + suffix
	semesterID := "integration-semester-" + suffix
	lessonID := "integration-lesson-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, cleanup := range []struct {
			query string
			arg   string
		}{
			{`DELETE FROM worker_status WHERE name=$1`, workerName},
			{`DELETE FROM users WHERE id=$1`, userID},
			{`DELETE FROM universities WHERE id=$1`, universityID},
		} {
			if _, cleanupErr := db.ExecContext(cleanupCtx, cleanup.query, cleanup.arg); cleanupErr != nil {
				t.Errorf("cleanup integration fixture %q: %v", cleanup.arg, cleanupErr)
			}
		}
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})

	universities := repository.NewUniversityRepository(db)
	groups := repository.NewGroupRepository(db)
	users := repository.NewUserRepository(db)
	subscriptions := repository.NewSubscriptionRepository(db)
	reminders := repository.NewReminderRepository(db)
	workers := repository.NewWorkerStatusRepository(db)
	dataSources := repository.NewDataSourceRepository(db)
	parseLogs := repository.NewParseLogRepository(db)
	snapshots := repository.NewParserSnapshotRepository(db)
	notifications := repository.NewNotificationRepository(db)
	if _, err = notifications.ClaimPending(ctx, 1); err != nil {
		t.Fatalf("claim pending notifications: %v", err)
	}
	if _, err = notifications.ClaimBotOutbox(ctx, 1); err != nil {
		t.Fatalf("claim bot outbox: %v", err)
	}

	if _, err = universities.CreateUniversity(
		ctx,
		universityID,
		"Integration University "+suffix,
		"Integration University",
		"https://example.test/schedule",
		true,
	); err != nil {
		t.Fatalf("create university: %v", err)
	}
	if _, err = groups.CreateGroup(ctx, groupID, universityID, "TEST-"+suffix, true); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err = dataSources.CreateDataSource(
		ctx,
		dataSourceID,
		universityID,
		"integration",
		"{}",
		3600,
	); err != nil {
		t.Fatalf("create data source: %v", err)
	}
	if _, err = parseLogs.CreateParseLog(ctx, parseLogID, dataSourceID, "running", 0, ""); err != nil {
		t.Fatalf("create parse log: %v", err)
	}
	periodStart := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	trustedSnapshot := &domain.ParserSnapshot{
		ID:           snapshotID,
		DataSourceID: dataSourceID,
		ParseLogID:   parseLogID,
		Status:       domain.SnapshotStatusStaged,
		Publishable:  true,
		GroupCount:   1,
		LessonCount:  1,
		Payload: domain.ScheduleSnapshot{
			UniversityID: universityID,
			SemesterID:   semesterID,
			StartDate:    periodStart,
			EndDate:      periodEnd,
			Groups: []domain.SnapshotGroup{{
				ID:           groupID,
				UniversityID: universityID,
				Name:         "TEST-" + suffix,
				Lessons: []domain.Lesson{{
					ID:           lessonID,
					UniversityID: universityID,
					SemesterID:   semesterID,
					DayOfWeek:    1,
					TimeStart:    "08:00",
					TimeEnd:      "09:35",
					WeekType:     domain.WeekTypeEvery,
					Subject:      "Integration lesson",
					Type:         domain.LessonTypeLecture,
					GroupID:      groupID,
					ValidFrom:    &periodStart,
					ValidTo:      &periodEnd,
				}},
			}},
		},
	}
	if err = snapshots.Create(ctx, trustedSnapshot); err != nil {
		t.Fatalf("create parser snapshot: %v", err)
	}
	if _, err = snapshots.Publish(ctx, snapshotID, "integration", "approved"); err != nil {
		t.Fatalf("publish parser snapshot: %v", err)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM lessons WHERE id=$1`, lessonID); err != nil {
		t.Fatalf("alter working schedule after publication: %v", err)
	}
	baseline, err := snapshots.Baseline(ctx, universityID, dataSourceID)
	if err != nil {
		t.Fatalf("load trusted snapshot baseline: %v", err)
	}
	if baseline.CurrentSnapshot != snapshotID || baseline.TrustedSnapshot == nil ||
		baseline.LessonCount != 1 || baseline.LessonsByGroup[groupID] != 1 {
		t.Fatalf("baseline was not restored from the trusted snapshot: %+v", baseline)
	}
	rejectedSnapshot := *trustedSnapshot
	rejectedSnapshot.ID = "integration-rejected-snapshot-" + suffix
	rejectedSnapshot.Status = domain.SnapshotStatusStaged
	if err = snapshots.Create(ctx, &rejectedSnapshot); err != nil {
		t.Fatalf("create staged snapshot to reject: %v", err)
	}
	if err = snapshots.Reject(ctx, rejectedSnapshot.ID, "integration", "invalid test candidate"); err != nil {
		t.Fatalf("reject staged snapshot: %v", err)
	}
	rejected, err := snapshots.Get(ctx, rejectedSnapshot.ID)
	if err != nil || rejected == nil || rejected.Status != domain.SnapshotStatusRejected {
		t.Fatalf("staged snapshot was not rejected: snapshot=%+v err=%v", rejected, err)
	}
	queuedSnapshotID := "integration-queued-snapshot-" + suffix
	for index := range 22 {
		candidate := *trustedSnapshot
		candidate.ID = fmt.Sprintf("integration-retention-snapshot-%02d-%s", index, suffix)
		candidate.Status = domain.SnapshotStatusStaged
		if index == 0 {
			candidate.ID = queuedSnapshotID
		}
		if err = snapshots.Create(ctx, &candidate); err != nil {
			t.Fatalf("create retention snapshot %d: %v", index, err)
		}
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (university_id, snapshot_id, reason)
		VALUES ($1,$2,'retention integration test')`, universityID, queuedSnapshotID); err != nil {
		t.Fatalf("queue snapshot protected from pruning: %v", err)
	}
	if err = snapshots.Prune(ctx, dataSourceID, snapshotID); err != nil {
		t.Fatalf("prune snapshot history with a pending reconciliation: %v", err)
	}
	var queuedSnapshotExists bool
	if err = db.GetContext(ctx, &queuedSnapshotExists,
		`SELECT EXISTS (SELECT 1 FROM parser_snapshots WHERE id=$1)`, queuedSnapshotID); err != nil {
		t.Fatal(err)
	}
	if !queuedSnapshotExists {
		t.Fatal("pruning removed a snapshot required by pending reconciliation")
	}
	if _, err = db.ExecContext(ctx,
		`DELETE FROM publication_reconciliation_queue WHERE university_id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (
			university_id, snapshot_id, reason, claim_token, claimed_at
		)
		VALUES ($1,$2,'stale claim integration test','terminated-worker',
		        NOW() - INTERVAL '31 minutes')`, universityID, snapshotID); err != nil {
		t.Fatalf("queue reconciliation with a stale claim: %v", err)
	}
	reconciled, err := snapshots.ReconcilePendingPublications(ctx)
	if err != nil {
		t.Fatalf("reclaim stale publication reconciliation: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("stale publication reconciliation count=%d, want 1", reconciled)
	}
	var restoredLessonExists bool
	if err = db.GetContext(ctx, &restoredLessonExists,
		`SELECT EXISTS (SELECT 1 FROM lessons WHERE id=$1)`, lessonID); err != nil {
		t.Fatal(err)
	}
	if !restoredLessonExists {
		t.Fatal("stale reconciliation claim did not restore the published schedule")
	}

	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (university_id, snapshot_id, reason)
		VALUES ($1,$2,'failed recovery integration test')`, universityID, queuedSnapshotID); err != nil {
		t.Fatalf("queue invalid publication reconciliation: %v", err)
	}
	reconciled, err = snapshots.ReconcilePendingPublications(ctx)
	if err == nil {
		t.Fatal("reconciliation of a non-published snapshot must fail")
	}
	if reconciled != 0 {
		t.Fatalf("failed publication reconciliation count=%d, want 0", reconciled)
	}
	var failedClaim struct {
		Attempts   int    `db:"attempts"`
		ClaimToken string `db:"claim_token"`
		LastError  string `db:"last_error"`
	}
	if err = db.GetContext(ctx, &failedClaim, `
		SELECT attempts, claim_token, last_error
		FROM publication_reconciliation_queue
		WHERE university_id=$1`, universityID); err != nil {
		t.Fatalf("load failed publication reconciliation: %v", err)
	}
	if failedClaim.Attempts != 1 || failedClaim.ClaimToken != "" || failedClaim.LastError == "" {
		t.Fatalf("failed reconciliation was not released for retry: %+v", failedClaim)
	}
	if _, err = db.ExecContext(ctx,
		`DELETE FROM publication_reconciliation_queue WHERE university_id=$1`, universityID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION integration_steal_publication_claim()
		RETURNS trigger AS $$
		BEGIN
			UPDATE publication_reconciliation_queue
			SET claim_token='stolen-by-integration-test'
			WHERE snapshot_id=NEW.id AND reason='lost claim integration test';
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER integration_steal_publication_claim
		AFTER UPDATE ON parser_snapshots
		FOR EACH ROW EXECUTE FUNCTION integration_steal_publication_claim()`); err != nil {
		t.Fatalf("install lost-claim integration trigger: %v", err)
	}
	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (university_id, snapshot_id, reason)
		VALUES ($1,$2,'lost claim integration test')`, universityID, snapshotID); err != nil {
		t.Fatalf("queue lost-claim publication reconciliation: %v", err)
	}
	reconciled, err = snapshots.ReconcilePendingPublications(ctx)
	if err == nil || !strings.Contains(err.Error(), "claim was lost") {
		t.Fatalf("lost reconciliation claim was not detected: count=%d err=%v", reconciled, err)
	}
	if reconciled != 0 {
		t.Fatalf("lost-claim publication reconciliation count=%d, want 0", reconciled)
	}
	var stolenClaim struct {
		Attempts   int    `db:"attempts"`
		ClaimToken string `db:"claim_token"`
	}
	if err = db.GetContext(ctx, &stolenClaim, `
		SELECT attempts, claim_token
		FROM publication_reconciliation_queue
		WHERE university_id=$1`, universityID); err != nil {
		t.Fatalf("load stolen publication reconciliation claim: %v", err)
	}
	if stolenClaim.Attempts != 1 || stolenClaim.ClaimToken != "stolen-by-integration-test" {
		t.Fatalf("unexpected lost-claim state: %+v", stolenClaim)
	}
	if _, err = db.ExecContext(ctx,
		`DROP TRIGGER integration_steal_publication_claim ON parser_snapshots`); err != nil {
		t.Fatalf("remove lost-claim integration trigger: %v", err)
	}
	if _, err = db.ExecContext(ctx,
		`DROP FUNCTION integration_steal_publication_claim()`); err != nil {
		t.Fatalf("remove lost-claim integration function: %v", err)
	}
	if _, err = db.ExecContext(ctx,
		`DELETE FROM publication_reconciliation_queue WHERE university_id=$1`, universityID); err != nil {
		t.Fatalf("remove lost-claim integration queue item: %v", err)
	}
	if _, err = users.CreateUser(ctx, userID, "integration_user", false); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pendingMenus, err := users.GetUsersPendingMenuSync(ctx, "", 10_000, "admin:test", "commands:v1")
	if err != nil || !containsUser(pendingMenus, userID) {
		t.Fatalf("new user is absent from pending menu sync: found=%t err=%v", containsUser(pendingMenus, userID), err)
	}
	if err = users.MarkMenuConfigured(ctx, userID, "commands:v1"); err != nil {
		t.Fatalf("mark user menu configured: %v", err)
	}
	pendingMenus, err = users.GetUsersPendingMenuSync(ctx, "", 10_000, "admin:test", "commands:v1")
	if err != nil || containsUser(pendingMenus, userID) {
		t.Fatalf("configured user remains in pending menu sync: found=%t err=%v", containsUser(pendingMenus, userID), err)
	}
	if err = users.SetDefaultGroup(ctx, userID, groupID); err != nil {
		t.Fatalf("set default group: %v", err)
	}
	if err = users.SetLessonReminder(ctx, userID, true, 15); err != nil {
		t.Fatalf("enable reminders: %v", err)
	}
	if err = subscriptions.UpsertSubscription(
		ctx,
		subscriptionID,
		userID,
		groupID,
		"group",
	); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	storedUser, err := users.GetUserByID(ctx, userID)
	if err != nil || storedUser == nil {
		t.Fatalf("get user: user=%+v err=%v", storedUser, err)
	}
	if storedUser.DefaultGroupID != groupID || !storedUser.ReminderEnabled {
		t.Fatalf("unexpected stored user: %+v", storedUser)
	}
	items, err := subscriptions.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("get subscriptions: %v", err)
	}
	if len(items) != 1 || items[0].GroupID != groupID || !items[0].IsDefault {
		t.Fatalf("unexpected subscriptions: %+v", items)
	}
	if items[0].ScheduleViewFormat != domain.ScheduleViewCompact {
		t.Fatalf("new subscription view = %q, want compact", items[0].ScheduleViewFormat)
	}
	if err = subscriptions.SetGroupScheduleView(ctx, userID, groupID, domain.ScheduleViewVisual); err != nil {
		t.Fatalf("set visual subscription view: %v", err)
	}
	items, err = subscriptions.GetGroupSubscriptions(ctx, userID)
	if err != nil || len(items) != 1 || items[0].ScheduleViewFormat != domain.ScheduleViewVisual {
		t.Fatalf("visual subscription view was not saved: items=%+v err=%v", items, err)
	}
	recipients, err := reminders.ActiveRecipientsPage(ctx, "", 10_000)
	if err != nil {
		t.Fatalf("get reminder recipients: %v", err)
	}
	if !containsReminderRecipient(recipients, userID, groupID) {
		t.Fatalf("integration user is absent from reminder recipients")
	}

	startedAt := time.Now().UTC().Add(-time.Second)
	finishedAt := time.Now().UTC()
	if err = workers.RecordRun(ctx, domain.WorkerRunResult{
		Name:            workerName,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		LastFullCycleAt: &finishedAt,
		Processed:       1,
	}); err != nil {
		t.Fatalf("record worker run: %v", err)
	}
	workerStatus, err := workers.Get(ctx, workerName)
	if err != nil {
		t.Fatalf("get worker status: %v", err)
	}
	if workerStatus.LastFullCycleAt == nil || workerStatus.LastProcessed != 1 {
		t.Fatalf("unexpected worker status: %+v", workerStatus)
	}
}

func containsUser(items []domain.User, userID string) bool {
	for _, item := range items {
		if item.ID == userID {
			return true
		}
	}
	return false
}

func containsReminderRecipient(
	items []domain.ReminderRecipient,
	userID string,
	groupID string,
) bool {
	for _, item := range items {
		if item.UserID == userID && item.GroupID == groupID {
			return true
		}
	}
	return false
}
