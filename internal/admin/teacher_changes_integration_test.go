//go:build integration

package admin

import (
	"context"
	"os"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestEditorNotifiesBothTeachersOnReplacementAndDeletion(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Connect("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := "teacher-editor-" + uuid.NewString()
	defer db.Exec(`DELETE FROM universities WHERE id=$1`, id)
	defer db.Exec(`DELETE FROM users WHERE id IN ($1,$2)`, id+"old", id+"new")
	for _, query := range []string{
		`INSERT INTO universities(id,name) VALUES($1,'Teacher editor')`,
		`INSERT INTO groups(id,name,university_id) VALUES($1,'Group',$1)`,
		`INSERT INTO semesters(id,external_id,name,university_id,start_date,end_date) VALUES($1,$1,'Term',$1,'2026-09-01','2026-12-31')`,
		`INSERT INTO users(id) VALUES($1||'old'),($1||'new')`,
	} {
		if _, err = db.Exec(query, id); err != nil {
			t.Fatal(err)
		}
	}
	users := repository.NewUserRepository(db)
	if err = users.SetTeacher(ctx, id+"old", id, "Иванов И.И."); err != nil {
		t.Fatal(err)
	}
	if err = users.SetTeacher(ctx, id+"new", id, "Петров П.П."); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	mutation := LessonMutation{GroupID: id, SemesterID: id, DayOfWeek: 1, TimeStart: "09:00", TimeEnd: "10:30", WeekType: "every", Subject: "Math", Type: "lecture", Teacher: "Иванов И.И."}
	if _, err = store.CreateManualLesson(ctx, "synthetic", mutation); err != nil {
		t.Fatal(err)
	}
	count := func(want int) {
		t.Helper()
		var got int
		if e := db.Get(&got, `SELECT count(*) FROM bot_outbox WHERE user_id IN ($1,$2) AND kind='teacher_change'`, id+"old", id+"new"); e != nil || got != want {
			t.Fatalf("teacher changes=%d want=%d err=%v", got, want, e)
		}
	}
	count(1)
	schedule, err := store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson := schedule.Lessons[0]
	mutation.Teacher = "Петров П.П."
	if _, err = store.UpdateEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt, mutation); err != nil {
		t.Fatal(err)
	}
	count(3)
	schedule, err = store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson = schedule.Lessons[0]
	if _, err = store.UpdateEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt, mutation); err != nil {
		t.Fatal(err)
	}
	count(3)
	schedule, err = store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson = schedule.Lessons[0]
	if err = store.DeleteEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	count(4)
}
