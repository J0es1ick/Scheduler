//go:build integration

package admin

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestEditorNotifiesBothTeachersOnReplacementAndDeletion(t *testing.T) {
	t.Run("delivered changes remain separate", func(t *testing.T) {
		testEditorTeacherChanges(t, true)
	})
	t.Run("pending changes retain their original baseline", func(t *testing.T) {
		testEditorTeacherChanges(t, false)
	})
}

func testEditorTeacherChanges(t *testing.T, deliverBetweenChanges bool) {
	t.Helper()
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
	pending := func(user, before, after string) {
		t.Helper()
		var rows []struct {
			Context json.RawMessage `db:"schedule_context"`
		}
		if err := db.SelectContext(ctx, &rows, `SELECT schedule_context FROM bot_outbox
			WHERE user_id=$1 AND kind='teacher_change' AND status='pending'`, id+user); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("pending changes for %s: got %d want 1", user, len(rows))
		}
		var transition domain.ScheduleTransition
		if err := json.Unmarshal(rows[0].Context, &transition); err != nil {
			t.Fatal(err)
		}
		if transition.UniversityID != id {
			t.Fatalf("notification belongs to university %q, want %q", transition.UniversityID, id)
		}
		for _, state := range []struct {
			name    string
			lessons []domain.Lesson
			teacher string
		}{{"before", transition.Before, before}, {"after", transition.After, after}} {
			if state.teacher == "" {
				if len(state.lessons) != 0 {
					t.Fatalf("%s %s: unexpected lessons %+v", user, state.name, state.lessons)
				}
				continue
			}
			if len(state.lessons) != 1 || state.lessons[0].Teacher != state.teacher || state.lessons[0].GroupID != id {
				t.Fatalf("%s %s: got %+v, want one lesson by %s in group %s", user, state.name, state.lessons, state.teacher, id)
			}
		}
	}
	deliver := func() {
		t.Helper()
		if deliverBetweenChanges {
			if _, err := db.ExecContext(ctx, `UPDATE bot_outbox SET status='delivered', delivered_at=NOW()
				WHERE user_id IN ($1,$2) AND kind='teacher_change' AND status='pending'`, id+"old", id+"new"); err != nil {
				t.Fatal(err)
			}
		}
	}
	count(1)
	pending("old", "", "Иванов И.И.")
	deliver()
	schedule, err := store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson := schedule.Lessons[0]
	mutation.Teacher = "Петров П.П."
	if _, err = store.UpdateEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt, mutation); err != nil {
		t.Fatal(err)
	}
	want := 2
	oldBefore := ""
	if deliverBetweenChanges {
		want = 3
		oldBefore = "Иванов И.И."
	}
	count(want)
	pending("old", oldBefore, "")
	pending("new", "", "Петров П.П.")
	schedule, err = store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson = schedule.Lessons[0]
	if _, err = store.UpdateEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt, mutation); err != nil {
		t.Fatal(err)
	}
	count(want)
	pending("old", oldBefore, "")
	pending("new", "", "Петров П.П.")
	deliver()
	schedule, err = store.EditorSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lesson = schedule.Lessons[0]
	if err = store.DeleteEditorLesson(ctx, "synthetic", lesson.ID, lesson.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	newBefore := ""
	if deliverBetweenChanges {
		want = 4
		newBefore = "Петров П.П."
	} else {
		pending("old", "", "")
	}
	count(want)
	pending("new", newBefore, "")
}
