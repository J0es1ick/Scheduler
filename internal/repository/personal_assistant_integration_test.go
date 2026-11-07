//go:build integration

package repository_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/service"
	profileservice "github.com/J0es1ick/Scheduler/internal/service/profiles"
)

func TestAssistantPreferencesScheduleAndInvalidateQueuedMessages(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name,timezone) VALUES('u','University','Asia/Vladivostok');
    INSERT INTO groups(id,name,university_id) VALUES('g','Group','u');
    INSERT INTO users(id) VALUES('42');
    INSERT INTO subscriptions(id,user_id,object_id,object_type) VALUES('s','42','g','group');
    UPDATE users SET default_group_id='g' WHERE id='42'`)
	if err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	user, err := users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	preferences := user.NotificationPreferences()
	preferences.DailyEnabled = true
	preferences.DailyTime = "20:00"
	preferences.DailyDayOffset = 1
	preferences.DailyWeekdays = 1
	preferences.DailySkipEmpty = true
	if err = profileservice.NewUserService(users).SaveNotificationPreferences(ctx, "42", preferences); err != nil {
		t.Fatal(err)
	}
	if err = users.SaveNotificationPreferences(ctx, "42", preferences); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("stale settings: %v", err)
	}
	now := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	if _, err = db.ExecContext(ctx, `UPDATE users SET daily_next_at='2026-01-01' WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NewDailyRepository(db).EnqueueDue(ctx, now, 50); err != nil {
		t.Fatal(err)
	}
	var date string
	if err = db.GetContext(ctx, &date, `SELECT schedule_context->>'date' FROM bot_outbox WHERE kind='daily_schedule'`); err != nil || date != "2026-10-06" {
		t.Fatalf("tomorrow: %s %v", date, err)
	}
	user, err = users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.DailyNextAt == nil || user.DailyNextAt.UTC() != time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("next selected weekday: %v", user.DailyNextAt)
	}
	if _, err = db.ExecContext(ctx, `SELECT enqueue_schedule_change('e','g','parser','changed')`); err != nil {
		t.Fatal(err)
	}
	preferences = user.NotificationPreferences()
	preferences.PausedUntil = time.Now().AddDate(0, 0, 20).Format(time.DateOnly)
	if err = users.SaveNotificationPreferences(ctx, "42", preferences); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err = db.GetContext(ctx, &pending, `SELECT COUNT(*) FROM notification_queue_eligibility WHERE policy_decision='ready'`); err != nil || pending != 0 {
		t.Fatalf("pause eligibility: %d %v", pending, err)
	}
	user, err = users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.DailyNextAt == nil || user.DailyNextAt.Format(time.DateOnly) < preferences.PausedUntil {
		t.Fatalf("resume: %+v", user)
	}
}

func TestAssistantNotesRemainPrivateAndSurvivePublication(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('u','University');
    INSERT INTO groups(id,name,university_id) VALUES('g','Group','u'),('foreign','Other','u');
    INSERT INTO semesters(id,external_id,name,university_id,start_date,end_date) VALUES('term','term','Term','u','2026-09-01','2026-12-31');
    INSERT INTO users(id) VALUES('42'),('43');
    INSERT INTO subscriptions(id,user_id,object_id,object_type) VALUES('s','42','g','group'),('other','43','g','group');
    UPDATE users SET default_group_id='g';
    INSERT INTO lessons(id,university_id,group_id,semester_id,day_of_week,time_start,time_end,week_type,subject,type,teacher)
    VALUES('lesson','u','g','term',1,'09:00','10:30','every','Math','lecture','Иванов И.И.')`)
	if err != nil {
		t.Fatal(err)
	}
	schedules := service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), repository.NewGroupRepository(db))
	input := service.PersonalNoteInput{TargetID: "g", LessonID: "lesson", Date: "2026-10-05", Body: "Отчёт https://example.com", DueDate: "2026-10-09"}
	note, err := schedules.SavePersonalNote(ctx, "42", input)
	if err != nil {
		t.Fatal(err)
	}
	input.TargetID = "foreign"
	if _, err = schedules.SavePersonalNote(ctx, "42", input); err == nil {
		t.Fatal("created note for unowned target")
	}
	notes := repository.NewPersonalScheduleRepository(db)
	if err = notes.DeleteNote(ctx, "43", note.ID, note.Version); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("cross-user deletion: %v", err)
	}
	if _, err = schedules.SavePersonalNote(ctx, "43", service.PersonalNoteInput{ID: note.ID, Version: note.Version, Body: "stolen"}); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("cross-user update: %v", err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE lessons SET id='moved',day_of_week=2,time_start='11:00',time_end='12:30' WHERE id='lesson'`); err != nil {
		t.Fatal(err)
	}
	items, err := schedules.PersonalNotes(ctx, "42")
	if err != nil || len(items) != 1 || items[0].Binding != "needs_review" {
		t.Fatalf("moved note %+v %v", items, err)
	}
	note, err = schedules.SavePersonalNote(ctx, "42", service.PersonalNoteInput{ID: note.ID, Version: note.Version, TargetID: "g", LessonID: "moved", Date: "2026-10-06", Body: note.Body, DueDate: "2026-10-09", Rebind: true})
	if err != nil || note.LessonDate.Format(time.DateOnly) != "2026-10-06" {
		t.Fatalf("explicit rebind: %+v %v", note, err)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM lessons; UPDATE users SET default_group_id=NULL WHERE id='42'; DELETE FROM subscriptions WHERE user_id='42'`); err != nil {
		t.Fatal(err)
	}
	items, err = schedules.PersonalNotes(ctx, "42")
	if err != nil || len(items) != 1 || items[0].Binding != "needs_review" || items[0].DueDate == nil {
		t.Fatalf("removed lesson %+v %v", items, err)
	}
	updated, err := schedules.SavePersonalNote(ctx, "42", service.PersonalNoteInput{ID: note.ID, Version: note.Version, Body: note.Body, DueDate: "2026-10-09", Completed: true})
	if err != nil || !updated.Completed {
		t.Fatalf("complete archived: %+v %v", updated, err)
	}
	users := repository.NewUserRepository(db)
	exported, err := users.ExportUserData(ctx, "42")
	if err != nil || len(exported.Notes) != 1 {
		t.Fatalf("privacy export: %+v %v", exported, err)
	}
	other, err := users.ExportUserData(ctx, "43")
	if err != nil || len(other.Notes) != 0 {
		t.Fatalf("other privacy export: %+v %v", other, err)
	}
	if err = users.DeleteUser(ctx, "42"); err != nil {
		t.Fatal(err)
	}
	remaining, err := notes.Notes(ctx, "42")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("privacy deletion: %+v %v", remaining, err)
	}
}

func TestGroupInvitationPreservesPrimaryAndConsent(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('u','University');
    INSERT INTO groups(id,name,university_id) VALUES('old','Old','u'),('new','New','u');
    INSERT INTO users(id,notifications_enabled) VALUES('42',FALSE);
    INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('old-sub','42','group','old');
    UPDATE users SET default_group_id='old' WHERE id='42'`)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewSubscriptionRepository(db)
	if err = repo.AcceptGroupInvitation(ctx, "new-sub", "42", "new", 2, false); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	user, err := users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.DefaultGroupID != "old" || user.NotificationsEnabled || user.DailyEnabled || user.ServiceUpdatesConsent != nil {
		t.Fatalf("implicit changes: %+v", user)
	}
	if err = repo.AcceptGroupInvitation(ctx, "again", "42", "new", 1, true); err != nil {
		t.Fatal(err)
	}
	user, _ = users.GetUserByID(ctx, "42")
	if user.DefaultGroupID != "new" {
		t.Fatal("explicit primary not applied")
	}
	var count int
	if err = db.GetContext(ctx, &count, `SELECT count(*) FROM subscriptions WHERE user_id='42'`); err != nil || count != 2 {
		t.Fatalf("duplicate: %d %v", count, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET role='teacher' WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	if err = repo.AcceptGroupInvitation(ctx, "blocked", "42", "old", 0, true); err == nil {
		t.Fatal("teacher subscribed")
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET role='student' WHERE id='42'; UPDATE groups SET is_active=FALSE WHERE id='old'`); err != nil {
		t.Fatal(err)
	}
	if err = repo.AcceptGroupInvitation(ctx, "inactive", "42", "old", 0, true); err == nil {
		t.Fatal("inactive group accepted")
	}
}

func TestTeacherTransitionQueueCoalescesWithoutLosingBaseline(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('u','University'); INSERT INTO groups(id,name,university_id) VALUES('g','Group','u'); INSERT INTO users(id) VALUES('42')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.NewUserRepository(db).SetTeacher(ctx, "42", "u", "Иванов И.И."); err != nil {
		t.Fatal(err)
	}
	date := time.Now().AddDate(0, 0, 1)
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	before := domain.Lesson{ID: "l", GroupID: "g", Subject: "Math", Teacher: "Иванов И.И.", WeekType: domain.WeekTypeDate, SpecialDate: &date, TimeStart: "09:00", TimeEnd: "10:30", Room: "214"}
	middle, after := before, before
	middle.Room = "308"
	after.Room = "420"
	key := searchtext.TokenKey("Иванов И.И.")
	for _, pair := range [][2]domain.Lesson{{before, middle}, {middle, after}} {
		tx, e := db.BeginTxx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		e = repository.EnqueueTeacherChanges(ctx, tx, "u", map[string]string{key: "changed"}, []domain.Lesson{pair[0]}, []domain.Lesson{pair[1]})
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = db.ExecContext(ctx, `UPDATE bot_outbox SET next_attempt_at=clock_timestamp()-INTERVAL '1 minute' WHERE kind='teacher_change'`); err != nil {
		t.Fatal(err)
	}
	queue, err := repository.NewNotificationRepository(db).ClaimBotOutbox(ctx, 10)
	if err != nil || len(queue) != 1 {
		var diagnostics []struct {
			ID       string
			Policy   string `db:"policy_reason"`
			Decision string `db:"policy_decision"`
		}
		_ = db.SelectContext(ctx, &diagnostics, `SELECT id,policy_reason,policy_decision FROM notification_queue_eligibility`)
		t.Fatalf("coalescing: %+v %v policy=%+v", queue, err, diagnostics)
	}
	var transition domain.ScheduleTransition
	if err = json.Unmarshal(queue[0].ScheduleContext, &transition); err != nil {
		t.Fatal(err)
	}
	text, _, err := service.DescribeScheduleTransition(transition, domain.PersonalTarget{Role: domain.RoleTeacher, Name: "Иванов И.И.", Timezone: "Europe/Moscow"}, nil, time.Now())
	if err != nil || !strings.Contains(text, "214 → 420") || strings.Contains(text, "308") {
		t.Fatalf("baseline: %q %v", text, err)
	}
}

func TestStudentTransitionsCoalesceAndRetainInactiveGroupChanges(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('u','University');
    INSERT INTO groups(id,name,university_id) VALUES('g','Group','u'); INSERT INTO users(id) VALUES('42');
    INSERT INTO subscriptions(id,user_id,object_id,object_type) VALUES('sub','42','g','group'); UPDATE users SET default_group_id='g' WHERE id='42'`)
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().AddDate(0, 0, 1)
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	original := domain.Lesson{ID: "l", GroupID: "g", Subject: "Math", WeekType: domain.WeekTypeDate, SpecialDate: &date, TimeStart: "09:00", TimeEnd: "10:00", Room: "214"}
	changed := original
	changed.Room = "308"
	makeEvent := func(id string, before, after domain.Lesson) domain.NotificationDelivery {
		t.Helper()
		tx, e := db.BeginTxx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		e = repository.EnqueueScheduleTransition(ctx, tx, id, "g", "parser", "changed", []domain.Lesson{before}, []domain.Lesson{after})
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
		var raw json.RawMessage
		if e = db.GetContext(ctx, &raw, `SELECT change_context FROM schedule_change_events WHERE id=$1`, id); e != nil {
			t.Fatal(e)
		}
		return domain.NotificationDelivery{ID: id, UserID: "42", GroupID: "g", ChangeContext: raw}
	}
	first, second := makeEvent("first", original, changed), makeEvent("second", changed, original)
	schedules := service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), repository.NewGroupRepository(db))
	items, err := schedules.DescribeNotifications(ctx, "42", []domain.NotificationDelivery{first, second})
	if err != nil || len(items) != 2 || items[0].Summary != "" || items[1].Summary != "" {
		t.Fatalf("reverted %+v %v", items, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE groups SET is_active=FALSE WHERE id='g'`); err != nil {
		t.Fatal(err)
	}
	items, err = schedules.DescribeNotifications(ctx, "42", []domain.NotificationDelivery{first})
	if err != nil || len(items) != 1 || !strings.Contains(items[0].Summary, "214 → 308") || len(items[0].Dates) != 1 {
		t.Fatalf("inactive %+v %v", items, err)
	}
}
