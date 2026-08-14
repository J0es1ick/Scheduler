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

func TestLessonIdentityKeepsOverrideAcrossSourceEditsAndTemporaryAbsence(t *testing.T) {
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
	universityID := "identity-university-" + suffix
	sourceID := "identity-source-" + suffix
	semesterID := "identity-semester-" + suffix
	groupID := "identity-group-" + suffix
	stableLessonID := "identity-lesson-original-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})
	if _, err = repository.NewUniversityRepository(db).CreateUniversity(
		ctx, universityID, "Identity test", "Identity test", "https://example.test", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NewDataSourceRepository(db).CreateDataSource(
		ctx, sourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}
	snapshots := repository.NewParserSnapshotRepository(db)
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	publish := func(snapshotID, incomingLessonID, subject, teacher, room string, includeLesson bool) {
		t.Helper()
		logID := snapshotID + "-log"
		if _, createErr := repository.NewParseLogRepository(db).CreateParseLog(
			ctx, logID, sourceID, "running", 0, "",
		); createErr != nil {
			t.Fatal(createErr)
		}
		lessons := []domain.Lesson{}
		if includeLesson {
			lessons = append(lessons, domain.Lesson{
				ID: incomingLessonID, UniversityID: universityID, SemesterID: semesterID,
				SourceID: sourceID, DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35",
				WeekType: domain.WeekTypeEvery, Subject: subject, Type: domain.LessonTypeLecture,
				Teacher: teacher, Room: room, GroupID: groupID, ValidFrom: &start, ValidTo: &end,
			})
		}
		candidate := &domain.ParserSnapshot{
			ID: snapshotID, DataSourceID: sourceID, ParseLogID: logID,
			Status: domain.SnapshotStatusStaged, Publishable: true,
			GroupCount: 1, LessonCount: len(lessons),
			Payload: domain.ScheduleSnapshot{
				UniversityID: universityID, SemesterID: semesterID,
				StartDate: start, EndDate: end,
				Groups: []domain.SnapshotGroup{{
					ID: groupID, UniversityID: universityID, Name: "IDENTITY-" + suffix,
					Lessons: lessons,
				}},
			},
		}
		if createErr := snapshots.Create(ctx, candidate); createErr != nil {
			t.Fatal(createErr)
		}
		if _, publishErr := snapshots.Publish(ctx, snapshotID, "integration", "identity test"); publishErr != nil {
			t.Fatal(publishErr)
		}
	}

	firstSnapshotID := "identity-snapshot-1-" + suffix
	publish(firstSnapshotID, stableLessonID, "Old subject", "Old teacher", "101", true)
	if _, err = db.ExecContext(ctx, `
		INSERT INTO lesson_overrides (
			id, base_lesson_id, university_id, semester_id, day_of_week,
			time_start, time_end, week_type, subject, type, teacher, room,
			group_id, subgroup, valid_from, valid_to, created_by
		) VALUES ($1,$2,$3,$4,1,'08:00','09:35','every','Manual subject','lecture',
		          'Manual teacher','M-1',$5,0,$6,$7,'integration')`,
		"identity-override-"+suffix, stableLessonID, universityID, semesterID, groupID, start, end); err != nil {
		t.Fatal(err)
	}
	publish("identity-snapshot-2-"+suffix, "changed-id-"+suffix,
		"Corrected subject", "Corrected teacher", "202", true)
	assertEffectiveOverride(t, ctx, db, groupID, stableLessonID, "changed-id-"+suffix)

	publish("identity-snapshot-3-"+suffix, "", "", "", "", false)
	var effectiveDuringAbsence int
	if err = db.GetContext(ctx, &effectiveDuringAbsence,
		`SELECT COUNT(*)::int FROM effective_lessons WHERE group_id=$1 AND subject='Manual subject'`, groupID); err != nil {
		t.Fatal(err)
	}
	if effectiveDuringAbsence != 1 {
		t.Fatalf("manual override disappeared with source lesson: count=%d", effectiveDuringAbsence)
	}

	latestSnapshotID := "identity-snapshot-4-" + suffix
	publish(latestSnapshotID, "returned-id-"+suffix,
		"Returned subject", "Returned teacher", "303", true)
	assertEffectiveOverride(t, ctx, db, groupID, stableLessonID, "returned-id-"+suffix)

	// Models migration 026 selecting A followed by an old application
	// publishing B before startup reconciliation. A is now obsolete and must
	// not overwrite B or block every subsequent process start.
	if _, err = db.ExecContext(ctx, `
		INSERT INTO publication_reconciliation_queue (university_id, snapshot_id, reason)
		VALUES ($1,$2,'rolling migration integration test')`, universityID, firstSnapshotID); err != nil {
		t.Fatal(err)
	}
	if reconciled, reconcileErr := snapshots.ReconcilePendingPublications(ctx); reconcileErr != nil || reconciled != 1 {
		t.Fatalf("reconcile stale migration publication: count=%d err=%v", reconciled, reconcileErr)
	}
	var currentSnapshotID string
	if err = db.GetContext(ctx, &currentSnapshotID,
		`SELECT current_snapshot_id FROM data_sources WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if currentSnapshotID != latestSnapshotID {
		t.Fatalf("stale reconciliation replaced newer snapshot: got=%s want=%s", currentSnapshotID, latestSnapshotID)
	}
}

func assertEffectiveOverride(
	t *testing.T,
	ctx context.Context,
	db *sqlx.DB,
	groupID, stableLessonID, rejectedLessonID string,
) {
	t.Helper()
	var state struct {
		Stable   int `db:"stable"`
		Rejected int `db:"rejected"`
		Manual   int `db:"manual"`
	}
	if err := db.GetContext(ctx, &state, `
		SELECT
			(SELECT COUNT(*)::int FROM lessons WHERE id=$1) AS stable,
			(SELECT COUNT(*)::int FROM lessons WHERE id=$2) AS rejected,
			(SELECT COUNT(*)::int FROM effective_lessons
			 WHERE group_id=$3 AND subject='Manual subject') AS manual`,
		stableLessonID, rejectedLessonID, groupID); err != nil {
		t.Fatal(err)
	}
	if state.Stable != 1 || state.Rejected != 0 || state.Manual != 1 {
		t.Fatalf("identity/override mismatch: %+v (stable=%s rejected=%s)",
			state, stableLessonID, rejectedLessonID)
	}
}
