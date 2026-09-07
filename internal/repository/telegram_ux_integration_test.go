//go:build integration

package repository_test

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

func TestTelegramUXFeedbackAndTimeGrid(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.Exec(`INSERT INTO users(id,is_admin,admin_role) VALUES('student',false,'none'),('admin',true,'owner');
   INSERT INTO universities(id,name) VALUES('one','One'),('two','Two');
   INSERT INTO semesters(id,university_id,external_id,name,start_date,end_date) VALUES('one-term','one','one-term','Term','2026-09-01','2026-12-31'),('two-term','two','two-term','Term','2026-09-01','2026-12-31');
   INSERT INTO groups(id,university_id,name,is_active) VALUES('active','one','A',true),('inactive','one','B',false),('foreign','two','C',true);
   INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type)
   VALUES('normal','one','one-term','active',1,'12:10','13:45','every','Normal','lecture'),('disabled','one','one-term','inactive',1,'11:00','12:30','every','Inactive','lecture'),('foreign','two','two-term','foreign',1,'10:00','11:30','every','Other university','lecture');`)
	if err != nil {
		t.Fatal(err)
	}
	support := service.NewSupportRequestService(repository.NewSupportRequestRepository(db))
	id, err := support.Submit(ctx, "student", domain.SupportRequestFeedback, "Хочу предложить настройку размера текста в боте")
	if err != nil {
		t.Fatal(err)
	}
	var kind, body string
	if err = db.QueryRowx(`SELECT r.request_type,o.body FROM support_requests r JOIN bot_outbox o ON o.request_id=r.id WHERE r.id=$1`, id).Scan(&kind, &body); err != nil {
		t.Fatal(err)
	}
	if kind != "feedback" || !strings.Contains(body, "Пожелания и обратная связь") {
		t.Fatalf("feedback not delivered to admin outbox: %s %s", kind, body)
	}
	if _, err = db.Exec(`INSERT INTO support_requests(id,user_id,request_type,details) VALUES('bad','student','unrecognized','This unsupported request must be rejected')`); err == nil {
		t.Fatal("constraint accepts unknown request types")
	}
	from := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	slots, err := repository.NewLessonRepository(db).GetTimeSlots(ctx, "one", from, from.AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].Start != "12:10" || slots[0].End != "13:45" {
		t.Fatalf("grid includes inactive/foreign data or loses nullable semester bounds: %+v", slots)
	}
	slots, err = repository.NewLessonRepository(db).GetTimeSlots(ctx, "one", from.AddDate(1, 0, 0), from.AddDate(1, 0, 6))
	if err != nil || len(slots) != 0 {
		t.Fatalf("expired grid: %+v %v", slots, err)
	}
}
