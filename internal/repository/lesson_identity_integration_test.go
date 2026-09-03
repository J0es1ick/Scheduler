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
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})
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
		if _, cleanupErr := db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID); cleanupErr != nil {
			t.Errorf("cleanup identity university: %v", cleanupErr)
		}
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

func TestLessonIdentityPublishesWhenSyntheticSlotOrderChanges(t *testing.T) {
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
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()
	universityID := "slot-university-" + suffix
	sourceID := "slot-source-" + suffix
	semesterID := "slot-semester-" + suffix
	groupID := "slot-group-" + suffix
	lessonAID := "slot-lesson-a-" + suffix
	lessonBID := "slot-lesson-b-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, cleanupErr := db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID); cleanupErr != nil {
			t.Errorf("cleanup slot university: %v", cleanupErr)
		}
	})
	if _, err = repository.NewUniversityRepository(db).CreateUniversity(
		ctx, universityID, "Slot test", "Slot test", "https://example.test", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NewDataSourceRepository(db).CreateDataSource(
		ctx, sourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	lesson := func(id, externalID, subject string) domain.Lesson {
		return domain.Lesson{
			ID: id, ExternalID: externalID, SourceID: sourceID,
			UniversityID: universityID, SemesterID: semesterID, GroupID: groupID,
			DayOfWeek: 2, TimeStart: "09:50", TimeEnd: "11:25",
			WeekType: domain.WeekTypeOdd, Subject: subject, Type: domain.LessonTypeLecture,
			ValidFrom: &start, ValidTo: &end,
		}
	}
	snapshots := repository.NewParserSnapshotRepository(db)
	publish := func(snapshotID string, lessons []domain.Lesson) {
		t.Helper()
		logID := snapshotID + "-log"
		if _, createErr := repository.NewParseLogRepository(db).CreateParseLog(
			ctx, logID, sourceID, "running", len(lessons), "",
		); createErr != nil {
			t.Fatal(createErr)
		}
		candidate := &domain.ParserSnapshot{
			ID: snapshotID, DataSourceID: sourceID, ParseLogID: logID,
			Status: domain.SnapshotStatusStaged, Publishable: true,
			GroupCount: 1, LessonCount: len(lessons),
			Payload: domain.ScheduleSnapshot{
				UniversityID: universityID, SemesterID: semesterID,
				StartDate: start, EndDate: end,
				Groups: []domain.SnapshotGroup{{
					ID: groupID, UniversityID: universityID, Name: "SLOT-" + suffix,
					Lessons: lessons,
				}},
			},
		}
		if createErr := snapshots.Create(ctx, candidate); createErr != nil {
			t.Fatal(createErr)
		}
		if _, publishErr := snapshots.Publish(ctx, snapshotID, "integration", "slot order test"); publishErr != nil {
			t.Fatal(publishErr)
		}
	}

	publish("slot-snapshot-1-"+suffix, []domain.Lesson{
		lesson(lessonAID, "slot:0", "Existing lesson"),
	})
	publish("slot-snapshot-2-"+suffix, []domain.Lesson{
		lesson(lessonBID, "slot:0", "New lesson"),
		lesson(lessonAID, "slot:1", "Existing lesson"),
	})
	publish("slot-snapshot-3-"+suffix, []domain.Lesson{
		lesson(lessonAID, "slot:0", "Existing lesson"),
	})

	var state struct {
		Lessons  int    `db:"lessons"`
		A        int    `db:"a"`
		B        int    `db:"b"`
		Slot0    int    `db:"slot0"`
		ASlot0   int    `db:"a_slot0"`
		ASubject string `db:"a_subject"`
	}
	if err = db.GetContext(ctx, &state, `
		SELECT
			(SELECT COUNT(*)::int FROM lessons WHERE group_id=$1) AS lessons,
			(SELECT COUNT(*)::int FROM lessons WHERE id=$2) AS a,
			(SELECT COUNT(*)::int FROM lessons WHERE id=$3) AS b,
			(SELECT COUNT(*)::int FROM lesson_source_identities
			 WHERE source_id=$4 AND semester_id=$5 AND external_id='slot:0' AND lesson_id=$3) AS slot0,
			(SELECT COUNT(*)::int FROM lesson_source_identities
			 WHERE source_id=$4 AND semester_id=$5 AND external_id='slot:0' AND lesson_id=$2) AS a_slot0,
			COALESCE((SELECT subject FROM lessons WHERE id=$2), '') AS a_subject`,
		groupID, lessonAID, lessonBID, sourceID, semesterID); err != nil {
		t.Fatal(err)
	}
	if state.Lessons != 1 || state.A != 1 || state.B != 0 || state.Slot0 != 0 || state.ASlot0 != 1 || state.ASubject != "Existing lesson" {
		t.Fatalf("synthetic slot reconciliation mismatch: %+v", state)
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
