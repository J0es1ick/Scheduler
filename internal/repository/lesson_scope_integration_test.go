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

func TestTeacherAndRoomQueriesAreScopedByUniversity(t *testing.T) {
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
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := uuid.NewString()
	firstUniversity := "lesson-scope-a-" + suffix
	secondUniversity := "lesson-scope-b-" + suffix
	universityRepo := repository.NewUniversityRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	semesterRepo := repository.NewSemesterRepository(db)
	lessonRepo := repository.NewLessonRepository(db)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, universityID := range []string{firstUniversity, secondUniversity} {
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
		}
	})
	for _, universityID := range []string{firstUniversity, secondUniversity} {
		if _, err = universityRepo.CreateUniversity(ctx, universityID, universityID, universityID, "", true); err != nil {
			t.Fatalf("create university %s: %v", universityID, err)
		}
		groupID := universityID + "-group"
		semesterID := universityID + "-semester"
		if _, err = groupRepo.CreateGroup(ctx, groupID, universityID, "TEST", true); err != nil {
			t.Fatalf("create group %s: %v", groupID, err)
		}
		from := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
		if _, err = semesterRepo.CreateSemester(ctx, semesterID, universityID, "2026", from, to); err != nil {
			t.Fatalf("create semester %s: %v", semesterID, err)
		}
		if err = lessonRepo.UpsertLesson(ctx, domain.Lesson{
			ID: universityID + "-lesson", UniversityID: universityID, SemesterID: semesterID,
			DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35", WeekType: domain.WeekTypeEvery,
			Subject: universityID, Type: domain.LessonTypeLecture, Teacher: "Общий преподаватель",
			Room: "А-101", GroupID: groupID, ValidFrom: &from, ValidTo: &to, UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("create lesson %s: %v", universityID, err)
		}
	}
	for _, query := range []struct {
		name string
		load func(context.Context, string, string) ([]domain.Lesson, error)
		term string
	}{
		{name: "teacher", load: lessonRepo.GetLessonsByTeacher, term: "Общий преподаватель"},
		{name: "room", load: lessonRepo.GetLessonsByRoom, term: "А-101"},
	} {
		t.Run(query.name, func(t *testing.T) {
			lessons, loadErr := query.load(ctx, firstUniversity, query.term)
			if loadErr != nil {
				t.Fatalf("load scoped lessons: %v", loadErr)
			}
			if len(lessons) != 1 || lessons[0].UniversityID != firstUniversity {
				t.Fatalf("lessons = %+v, want only university %s", lessons, firstUniversity)
			}
			if lessons[0].GroupName != "TEST" {
				t.Fatalf("group name = %q, want TEST", lessons[0].GroupName)
			}
		})
	}
}
