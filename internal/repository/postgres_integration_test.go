//go:build integration

package repository_test

import (
	"context"
	"os"
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
	if _, err = users.CreateUser(ctx, userID, "integration_user", false); err != nil {
		t.Fatalf("create user: %v", err)
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
