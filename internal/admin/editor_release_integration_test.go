//go:build integration

package admin

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestEditorSQLPreservesRecurrenceAcrossOverrideUpdates(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := "calendar-" + uuid.NewString()
	defer db.Exec(`DELETE FROM universities WHERE id=$1`, id)
	for _, q := range []string{
		`INSERT INTO universities(id,name) VALUES($1,'Synthetic calendar')`,
		`INSERT INTO groups(id,name,university_id) VALUES($1,'Test',$1)`,
		`INSERT INTO semesters(id,external_id,name,university_id,start_date,end_date) VALUES($1,$1,'Term',$1,'2026-09-01','2026-12-31')`,
		`INSERT INTO lessons(id,university_id,group_id,semester_id,day_of_week,time_start,time_end,week_type,subject,type,valid_from,valid_to,recurrence) VALUES($1,$1,$1,$1,3,'09:00','10:30','every','Physics','lecture','2026-09-02','2026-12-31','{"cycle_length":3,"cycle_weeks":[1,3],"anchor_date":"2026-09-01T00:00:00Z"}')`,
	} {
		if _, err = db.Exec(q, id); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(db)
	var expected domain.RecurrenceRule
	for _, room := range []string{"A-101", "A-202"} {
		schedule, err := store.EditorSchedule(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		lesson := schedule.Lessons[0]
		if expected.IsZero() {
			expected = lesson.Recurrence
		}
		if expected.IsZero() {
			t.Fatal("reading editor dropped recurrence")
		}
		mutation := LessonMutation{GroupID: id, SemesterID: id, DayOfWeek: lesson.DayOfWeek, TimeStart: lesson.TimeStart, TimeEnd: lesson.TimeEnd, WeekType: lesson.WeekType, Subject: lesson.Subject, Type: lesson.Type, Teacher: lesson.Teacher, Room: room, ValidFrom: lesson.ValidFrom, ValidTo: lesson.ValidTo}
		if _, err = store.UpdateEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt, mutation); err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"update", "delete"} {
			t.Run(room+"/stale_"+operation, func(t *testing.T) {
				var conflictErr error
				if operation == "update" {
					_, conflictErr = store.UpdateEditorLesson(ctx, "other-editor", lesson.ID, lesson.UpdatedAt, mutation)
				} else {
					conflictErr = store.DeleteEditorLesson(ctx, "other-editor", lesson.ID, lesson.UpdatedAt)
				}
				if !errors.Is(conflictErr, ErrConflict) {
					t.Fatalf("stale %s must report a version conflict, got %v", operation, conflictErr)
				}
			})
		}
		after, err := store.EditorSchedule(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expected, after.Lessons[0].Recurrence) || after.Lessons[0].Room != room {
			t.Fatal("override changed recurrence or lost edit")
		}
		if _, err = store.UpdateEditorLesson(ctx, "synthetic", "missing-"+id, lesson.UpdatedAt, mutation); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing lesson must remain not found, got %v", err)
		}
	}
}
