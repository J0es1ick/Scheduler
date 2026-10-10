//go:build integration

package repository_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"strings"
	"testing"
	"time"
)

func TestPersonalScheduleIsolationRecurrenceAndQueue(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.Exec(`INSERT INTO universities(id,name,timezone) VALUES('p','Personal','Europe/Moscow');
 INSERT INTO groups(id,university_id,name) VALUES('p','p','Group');
 INSERT INTO semesters(id,external_id,university_id,name,start_date,end_date) VALUES('p','p','p','Term','2026-09-01','2026-12-31');
 INSERT INTO users(id) VALUES('41'),('42');
 INSERT INTO subscriptions(id,user_id,object_id,object_type) VALUES('p1','41','p','group'),('p2','42','p','group');
 UPDATE users SET default_group_id='p';
 INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,teacher,room,valid_from,valid_to,recurrence)
 VALUES('weekly','p','p','p',1,'09:00','10:30','every','Physics','lecture','Иванов И.И.','100','2026-09-01','2026-12-31','{}'),
 ('biweekly','p','p','p',1,'11:00','12:30','every','Math','practice','Иванов И.И.','100','2026-09-01','2026-12-31','{"cycle_length":2,"cycle_weeks":[1],"anchor_date":"2026-09-07T00:00:00Z"}');`)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), repository.NewGroupRepository(db))
	repo := repository.NewPersonalScheduleRepository(db)
	date := func(raw string) time.Time {
		v, e := time.Parse(time.DateOnly, raw)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	save := func(user, lesson, day, scope string, patch domain.PersonalLessonPatch, cancel bool) *domain.PersonalOverride {
		t.Helper()
		v, e := scheduler.SavePersonalChange(ctx, user, service.PersonalChangeInput{TargetID: "p", LessonID: lesson, Date: day, Scope: scope, Patch: patch, Cancelled: cancel})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	load := func(user, day string) []domain.Lesson {
		t.Helper()
		d := date(day)
		base, e := scheduler.GetScheduleForGroupRange(ctx, "p", d, d)
		if e != nil {
			t.Fatal(e)
		}
		result, e := scheduler.PersonalizeSchedule(ctx, user, "p", "p", "", base)
		if e != nil {
			t.Fatal(e)
		}
		return result[d]
	}
	room := "Личная 202"
	semester := save("41", "weekly", "2026-09-07", "semester", domain.PersonalLessonPatch{Room: &room}, false)
	if got := load("41", "2026-09-14"); len(got) != 1 || got[0].Room != room {
		t.Fatalf("personal: %+v", got)
	}
	if got := load("42", "2026-09-14"); len(got) != 1 || got[0].Room != "100" {
		t.Fatalf("leaked to another user: %+v", got)
	}
	cancellation := save("41", "weekly", "2026-09-14", "day", domain.PersonalLessonPatch{}, true)
	if got := load("41", "2026-09-14"); len(got) != 0 {
		t.Fatalf("cancellation ignored: %+v", got)
	}
	if got := load("41", "2026-09-21"); len(got) != 2 || got[0].Room != room {
		t.Fatalf("one-day leaked: %+v", got)
	}
	if err = repo.Delete(ctx, "42", cancellation.ID, 1); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("foreign delete: %v", err)
	}
	if err = repo.Delete(ctx, "41", cancellation.ID, 2); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err = repo.Delete(ctx, "41", cancellation.ID, 1); err != nil {
		t.Fatal(err)
	}
	save("41", "biweekly", "2026-09-21", "semester", domain.PersonalLessonPatch{}, true)
	if got := load("41", "2026-10-05"); len(got) != 1 {
		t.Fatalf("biweekly cancellation: %+v", got)
	}
	if got := load("41", "2026-09-07"); len(got) != 2 {
		t.Fatalf("changed earlier date: %+v", got)
	}
	if _, err = scheduler.SavePersonalChange(ctx, "41", service.PersonalChangeInput{TargetID: "foreign", LessonID: "weekly", Date: "2026-09-07", Scope: "day"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign target: %v", err)
	}
	if _, err = db.Exec(`UPDATE lessons SET room='Changed upstream',teacher='Петров П.П.' WHERE id='weekly'`); err != nil {
		t.Fatal(err)
	}
	got := load("41", "2026-09-14")
	if len(got) != 1 || got[0].Room != room || got[0].Teacher != "Петров П.П." {
		t.Fatalf("upstream coexistence: %+v", got)
	}
	users := repository.NewUserRepository(db)
	if err = users.SetTeacher(ctx, "41", "p", "Петров П.П."); err != nil {
		t.Fatal(err)
	}
	if got := load("41", "2026-09-14"); len(got) != 1 || got[0].Room != "Changed upstream" {
		t.Fatalf("inactive role applied: %+v", got)
	}
	targets, err := scheduler.PersonalTargets(ctx, "41")
	if err != nil || len(targets) != 1 {
		t.Fatalf("teacher targets: %+v %v", targets, err)
	}
	teacherChange, err := scheduler.SavePersonalChange(ctx, "41", service.PersonalChangeInput{TargetID: targets[0].ID, LessonID: "weekly", Date: "2026-09-14", Scope: "day", Cancelled: true})
	if err != nil {
		t.Fatal(err)
	}
	base, err := scheduler.GetScheduleForTeacherRange(ctx, "p", "Петров П.П.", date("2026-09-14"), date("2026-09-14"))
	if err != nil {
		t.Fatal(err)
	}
	personal, err := scheduler.PersonalizeSchedule(ctx, "41", "", "p", "Петров П.П.", base)
	if err != nil || len(personal[date("2026-09-14")]) != 0 {
		t.Fatalf("teacher cancellation: %+v %v", personal, err)
	}
	if len(base[date("2026-09-14")]) != 1 {
		t.Fatal("shared schedule was mutated")
	}
	if err = repo.Delete(ctx, "41", teacherChange.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET role='student' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if got := load("41", "2026-09-14"); len(got) != 1 || got[0].Room != room {
		t.Fatalf("student data lost: %+v", got)
	}
	if _, err = scheduler.SavePersonalChange(ctx, "41", service.PersonalChangeInput{ID: semester.ID, Version: 1, TargetID: "p", LessonID: "weekly", Date: "2026-09-07", Scope: "semester", Cancelled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = scheduler.SavePersonalChange(ctx, "41", service.PersonalChangeInput{ID: semester.ID, Version: 1, TargetID: "p", LessonID: "weekly", Date: "2026-09-07", Scope: "semester", Cancelled: true}); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatalf("stale update: %v", err)
	}
	save("41", "weekly", "2026-09-14", "day", domain.PersonalLessonPatch{}, false)
	if got := load("41", "2026-09-14"); len(got) != 1 {
		t.Fatalf("day restore cancelled semester: %+v", got)
	}
	if err = users.SetDailySchedule(ctx, "41", true, "00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET daily_next_at=NOW()-INTERVAL '1 minute' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	daily := repository.NewDailyRepository(db)
	if _, err = daily.EnqueueDue(ctx, time.Now(), 10); err != nil {
		t.Fatal(err)
	}
	save("41", "weekly", "2026-09-28", "day", domain.PersonalLessonPatch{Room: &room}, false)
	var stale int
	if err = db.Get(&stale, `SELECT count(*) FROM bot_outbox WHERE user_id='41' AND kind='daily_schedule' AND status='pending' AND cancel_requested_at IS NULL`); err != nil || stale != 0 {
		t.Fatalf("stale queue: %d %v", stale, err)
	}
	if _, err = daily.EnqueueDue(ctx, time.Now(), 10); err != nil {
		t.Fatal(err)
	}
	if err = db.Get(&stale, `SELECT count(*) FROM bot_outbox WHERE user_id='41' AND kind='daily_schedule' AND status='pending' AND cancel_requested_at IS NULL`); err != nil || stale != 1 {
		t.Fatalf("queue not rebuilt: %d %v", stale, err)
	}
	exported, err := users.ExportUserData(ctx, "41")
	if err != nil || len(exported.PersonalChanges) < 3 {
		t.Fatalf("export: %+v %v", exported, err)
	}
	if _, err = db.Exec(`DELETE FROM users WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	changes, err := repo.List(ctx, "41")
	if err != nil || len(changes) != 0 {
		t.Fatalf("deletion retained data: %+v %v", changes, err)
	}
}

func TestPersonalPublicationReviewAndSelectedRepeats(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createRepositoryPublicationFixture(t, ctx, db, "review", "source")
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	snapshots := repository.NewParserSnapshotRepository(db)
	publish := func(id, subject string, remove bool) {
		t.Helper()
		lessons := []domain.Lesson{}
		for week := 0; week < 12; week++ {
			if remove && week == 2 {
				continue
			}
			day := start.AddDate(0, 0, week*7)
			lessons = append(lessons, domain.Lesson{ID: fmt.Sprintf("%s-%d", id, week), UniversityID: "review", SemesterID: "term", GroupID: "group", SpecialDate: &day, WeekType: domain.WeekTypeDate, TimeStart: "09:00", TimeEnd: "10:30", Subject: subject, Type: domain.LessonTypeLecture, Room: "100"})
		}
		if _, err := repository.NewParseLogRepository(db).CreateParseLog(ctx, id+"-log", "source", "running", 0, ""); err != nil {
			t.Fatal(err)
		}
		snapshot := domain.ParserSnapshot{ID: id, DataSourceID: "source", ParseLogID: id + "-log", Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1, LessonCount: len(lessons), Payload: domain.ScheduleSnapshot{UniversityID: "review", SemesterID: "term", StartDate: start, EndDate: start.AddDate(0, 0, 12*7-1), Groups: []domain.SnapshotGroup{{ID: "group", UniversityID: "review", Name: "4/147", Lessons: lessons}}}}
		if err := snapshots.Create(ctx, &snapshot); err != nil {
			t.Fatal(err)
		}
		if _, err := snapshots.Publish(ctx, id, "test", ""); err != nil {
			t.Fatal(err)
		}
	}
	publish("first", "Physics", false)
	if _, err := db.Exec(`INSERT INTO users(id) VALUES('review-user'); INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('review-sub','review-user','group','group'); UPDATE users SET default_group_id='group' WHERE id='review-user'`); err != nil {
		t.Fatal(err)
	}
	scheduler := service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), repository.NewGroupRepository(db))
	load := func() *service.PersonalSchedule {
		t.Helper()
		v, err := scheduler.PersonalSchedule(ctx, "review-user", "group", start, start)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	schedule := load()
	if len(schedule.Patterns) != 1 || schedule.Patterns[0].Kind != "weekly" || len(schedule.Days[0].Lessons[0].Repeats) != 12 {
		t.Fatalf("detection: %+v", schedule)
	}
	input := service.PersonalChangeInput{TargetID: "group", LessonID: schedule.Days[0].Lessons[0].Original.PersonalKey, Date: "2026-09-07", Scope: "selected", Dates: []string{"2026-09-07", "2026-09-21"}, Patch: domain.PersonalLessonPatch{Room: new("Personal")}}
	change, err := scheduler.SavePersonalChange(ctx, "review-user", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Dates = []string{"2026-09-08"}
	if _, err = scheduler.SavePersonalChange(ctx, "review-user", input); !errors.Is(err, service.ErrPersonalInput) {
		t.Fatal("invalid date accepted", err)
	}
	publish("identical", "Physics", false)
	if schedule = load(); schedule.Review != nil || schedule.Days[0].Lessons[0].Lesson.Room != "Personal" {
		t.Fatal("identical publication suspended edits", schedule.Review)
	}
	publish("changed", "Chemistry", true)
	schedule = load()
	if schedule.Review == nil || schedule.Review.Kept != 1 || schedule.Review.Dropped != 1 || schedule.Days[0].Lessons[0].Lesson.Room != "100" {
		t.Fatalf("review: %+v", schedule)
	}
	notice, err := repository.NewNotificationRepository(db).PersonalNotification(ctx, "review-user", "group", "Changed")
	if err != nil || !strings.Contains(notice, "согласовать") {
		t.Fatal("missing review notice", notice, err)
	}
	resolve := service.PersonalReviewInput{TargetID: "group", Publication: schedule.Publication, Action: "keep", Versions: map[string]int64{change.ID: schedule.Review.Items[0].Version}}
	stale := resolve
	stale.Publication = "first"
	if _, err = scheduler.ResolvePersonalReview(ctx, "review-user", stale); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatal("stale publication accepted", err)
	}
	if _, err = scheduler.ResolvePersonalReview(ctx, "review-user", resolve); err != nil {
		t.Fatal(err)
	}
	if _, err = scheduler.ResolvePersonalReview(ctx, "review-user", resolve); !errors.Is(err, repository.ErrPersonalConflict) {
		t.Fatal("double confirmation", err)
	}
	schedule = load()
	if schedule.Review != nil || schedule.Days[0].Lessons[0].Lesson.Room != "Personal" || schedule.Days[0].Lessons[0].Lesson.Subject != "Chemistry" {
		t.Fatal("remap failed", schedule)
	}
	var occurrences []domain.PersonalOccurrence
	if err = json.Unmarshal(schedule.Changes[0].Occurrences, &occurrences); err != nil || len(occurrences) != 1 {
		t.Fatal("lost selected dates", occurrences, err)
	}
	publish("again", "New subject", false)
	schedule = load()
	resolve.Publication = schedule.Publication
	resolve.Action = "discard"
	resolve.Versions[change.ID] = schedule.Review.Items[0].Version
	if _, err = scheduler.ResolvePersonalReview(ctx, "review-user", resolve); err != nil {
		t.Fatal(err)
	}
	if schedule = load(); len(schedule.Changes) != 0 || schedule.Review != nil {
		t.Fatal("discard failed", schedule)
	}
}
