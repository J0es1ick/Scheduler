//go:build integration

package repository_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
)

func TestTeacherProfileAndDailyQueue(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name,timezone) VALUES('roles','Roles','Europe/Moscow');
		INSERT INTO groups(id,name,university_id) VALUES('roles-group','Group','roles');
		INSERT INTO users(id,username) VALUES('42','test');
		INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('sub','42','group','roles-group');
		UPDATE users SET default_group_id='roles-group' WHERE id='42'`)
	if err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	if err = users.SetTeacher(ctx, "42", "roles", "Сизёва О. В."); err != nil {
		t.Fatal(err)
	}
	first, err := users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if first.Role != domain.RoleTeacher || first.DefaultGroupID != "roles-group" || first.TeacherID == "" {
		t.Fatalf("profile %#v", first)
	}
	if err = users.SetTeacher(ctx, "42", "roles", "Петров П.П."); err != nil {
		t.Fatal(err)
	}
	user, err := users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if first.TeacherID == user.TeacherID {
		t.Fatal("teacher was not replaced")
	}
	var subscriptions int
	if err = db.GetContext(ctx, &subscriptions, `SELECT count(*) FROM subscriptions WHERE user_id='42'`); err != nil || subscriptions != 1 {
		t.Fatalf("subscriptions %d: %v", subscriptions, err)
	}
	if err = users.SetDailySchedule(ctx, "42", true, "06:00"); err != nil {
		t.Fatal(err)
	}
	user, err = users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.DailySetup != "done" || user.DailyNextAt == nil || !user.DailyNextAt.After(time.Now()) {
		t.Fatalf("daily settings %#v", user)
	}
	_, err = db.ExecContext(ctx, `UPDATE users SET quiet_hours_enabled=TRUE,quiet_hours_start='00:00',quiet_hours_end='23:59',daily_next_at=NOW()-INTERVAL '1 minute' WHERE id='42'`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	location, _ := time.LoadLocation("Europe/Moscow")
	local := now.In(location)
	if local.Hour() < 6 {
		now = time.Date(local.Year(), local.Month(), local.Day(), 7, 0, 0, 0, location)
	}
	daily := repository.NewDailyRepository(db)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := daily.EnqueueDue(ctx, now, 250); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	queue := repository.NewNotificationRepository(db)
	items, err := queue.ClaimBotOutbox(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != "daily_schedule" || items[0].TeacherID != user.TeacherID || items[0].GroupID != "" {
		t.Fatalf("daily items %#v", items)
	}
	var dailyContext domain.DailyContext
	if err = json.Unmarshal(items[0].ScheduleContext, &dailyContext); err != nil || dailyContext.Date != now.In(location).Format(time.DateOnly) {
		t.Fatalf("daily context %#v %v", dailyContext, err)
	}
	if err = users.SetRole(ctx, "42", domain.RoleStudent); err != nil {
		t.Fatal(err)
	}
	decision, err := queue.BotOutboxDecision(ctx, items[0].ID, items[0].ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("role cancellation %s %v", decision, err)
	}
	if err = users.SetRole(ctx, "42", domain.RoleTeacher); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET quiet_hours_enabled=FALSE WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `SELECT enqueue_teacher_change('changed','roles',$1,'changed')`, searchtext.TokenKey("Петров П.П.")); err != nil {
		t.Fatal(err)
	}
	items, err = queue.ClaimBotOutbox(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != "teacher_change" {
		t.Fatalf("teacher notification %#v", items)
	}
	exported, err := users.ExportUserData(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	if exported.Teacher == nil || exported.Teacher.Name != "Петров П.П." {
		t.Fatalf("teacher export %#v", exported.Teacher)
	}
}

func TestDailyCatchupOnlyUsesCurrentUniversityDate(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name,timezone) VALUES('daily-zone','Zone','Asia/Vladivostok');
		INSERT INTO groups(id,name,university_id) VALUES('daily-group','Group','daily-zone');
		INSERT INTO users(id) VALUES('99');
		INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('daily-sub','99','group','daily-group');
		UPDATE users SET default_group_id='daily-group',daily_enabled=TRUE,daily_time='06:00' WHERE id='99';
		UPDATE users SET daily_next_at='2026-01-01 00:00Z' WHERE id='99'`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 3, 21, 0, 0, 0, time.UTC)
	repo := repository.NewDailyRepository(db)
	if _, err = repo.EnqueueDue(ctx, now, 250); err != nil {
		t.Fatal(err)
	}
	var dates []string
	if err = db.SelectContext(ctx, &dates, `SELECT schedule_context->>'date' FROM bot_outbox WHERE kind='daily_schedule'`); err != nil {
		t.Fatal(err)
	}
	if len(dates) != 1 || dates[0] != "2026-10-04" {
		t.Fatalf("dates %#v", dates)
	}
	if _, err = repo.EnqueueDue(ctx, now, 250); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.GetContext(ctx, &count, `SELECT count(*) FROM bot_outbox WHERE kind='daily_schedule'`); err != nil || count != 1 {
		t.Fatalf("count %d %v", count, err)
	}
}

func TestTeacherLessonsUseExactNormalizedIdentity(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('identity','Identity');
 INSERT INTO groups(id,name,university_id) VALUES('identity','Group','identity');
 INSERT INTO semesters(id,external_id,name,university_id,start_date,end_date) VALUES('identity','identity','Term','identity','2026-09-01','2026-12-31');
 INSERT INTO lessons(id,university_id,group_id,semester_id,day_of_week,time_start,time_end,week_type,subject,type,teacher)
 SELECT id,'identity','identity','identity',1,'09:00','10:00','every','Math','lecture',teacher FROM
 (VALUES('a','Сизёва О.В.; Иванов И.И.'),('b','Сизева О.А.'),('c','Сизев А.А.')) t(id,teacher)`)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewLessonRepository(db)
	lessons, err := repo.GetLessonsByTeacher(ctx, "identity", "СИЗЕВА О. В.")
	if err != nil || len(lessons) != 1 || lessons[0].ID != "a" {
		t.Fatalf("identity lessons %#v %v", lessons, err)
	}
	lessons, err = repo.GetLessonsByTeacher(ctx, "identity", "Сизев")
	if err != nil || len(lessons) != 0 {
		t.Fatalf("partial identity matched %#v %v", lessons, err)
	}
}

func TestDailyTimeChangeReplacesOnlyUnsentTask(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name,timezone) VALUES('replace-daily','Daily','Europe/Moscow');
 INSERT INTO groups(id,name,university_id) VALUES('replace-daily','Group','replace-daily');
 INSERT INTO users(id) VALUES('771');
 INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('replace-daily','771','group','replace-daily');
 UPDATE users SET default_group_id='replace-daily',daily_enabled=TRUE,daily_time='06:00' WHERE id='771';
 UPDATE users SET daily_next_at='2026-01-01' WHERE id='771'`)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("Europe/Moscow")
	date := time.Now().In(location)
	now := time.Date(date.Year(), date.Month(), date.Day(), 7, 0, 0, 0, location)
	repo := repository.NewDailyRepository(db)
	if _, err = repo.EnqueueDue(ctx, now, 250); err != nil {
		t.Fatal(err)
	}
	if err = repository.NewUserRepository(db).SetDailySchedule(ctx, "771", true, "08:00"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = db.GetContext(ctx, &status, `SELECT status FROM bot_outbox WHERE user_id='771'`); err != nil || status != "cancelled" {
		t.Fatalf("old task status %s %v", status, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET daily_next_at=$1 WHERE id='771'`, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.EnqueueDue(ctx, now.Add(time.Hour), 250); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &status, `SELECT status FROM bot_outbox WHERE user_id='771'`); err != nil || status != "pending" {
		t.Fatalf("new task status %s %v", status, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE bot_outbox SET schedule_messages='[{"text":"confirmed"}]',delivered_parts=1 WHERE user_id='771'`); err != nil {
		t.Fatal(err)
	}
	if err = repository.NewUserRepository(db).SetDailySchedule(ctx, "771", true, "09:00"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET daily_next_at=$1 WHERE id='771'`, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.EnqueueDue(ctx, now.Add(2*time.Hour), 250); err != nil {
		t.Fatal(err)
	}
	if err = db.GetContext(ctx, &status, `SELECT status FROM bot_outbox WHERE user_id='771'`); err != nil || status != "cancelled" {
		t.Fatalf("confirmed task revived: %s %v", status, err)
	}
}

func TestDailyExpiredMessageCannotBeClaimed(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES('expired-daily','Daily');
 INSERT INTO groups(id,name,university_id) VALUES('expired-daily','Group','expired-daily');
 INSERT INTO users(id) VALUES('expired-daily');
 INSERT INTO subscriptions(id,user_id,object_type,object_id) VALUES('expired-daily','expired-daily','group','expired-daily');
 UPDATE users SET default_group_id='expired-daily',daily_enabled=TRUE WHERE id='expired-daily';
 INSERT INTO bot_outbox(id,user_id,group_id,kind,body,expires_at) VALUES('expired-daily','expired-daily','expired-daily','daily_schedule','outdated',clock_timestamp()-INTERVAL '1 minute')`)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.NewNotificationRepository(db).ClaimBotOutbox(ctx, 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("expired delivery claimed: %#v %v", items, err)
	}
}
