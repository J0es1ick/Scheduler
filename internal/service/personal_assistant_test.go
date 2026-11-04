package service

import (
	"encoding/json"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"strings"
	"testing"
	"time"
)

func assistantDate(value string) time.Time { date, _ := time.Parse(time.DateOnly, value); return date }
func assistantLesson() domain.Lesson {
	date := assistantDate("2026-10-05")
	return domain.Lesson{ID: "lesson", PersonalKey: "lesson", GroupID: "group", GroupName: "4/147", SemesterID: "term", Subject: "Математика", Teacher: "Сизёва О.В.", Type: "lecture", TimeStart: "09:00", TimeEnd: "10:30", Room: "214", WeekType: domain.WeekTypeDate, SpecialDate: &date, DayOfWeek: 1}
}
func TestDetailedLessonChangesPreserveAmbiguityAndPriority(t *testing.T) {
	first := assistantLesson()
	room := first
	room.ID = "physics"
	room.Subject = "Физика"
	room.TimeStart = "11:00"
	room.TimeEnd = "12:30"
	changed := room
	changed.Room = "308"
	diff := DetailedLessonChanges([]domain.Lesson{first, room}, []domain.Lesson{changed})
	if len(diff) != 2 || diff[0].After != nil || !strings.Contains(strings.Join(diff[1].Fields, ""), "214 → 308") {
		t.Fatalf("changes: %+v", diff)
	}
	a, b := first, first
	a.ID = "a"
	b.ID = "b"
	b.TimeStart = "13:00"
	c, d := a, b
	c.ID = "c"
	d.ID = "d"
	c.TimeStart = "10:00"
	d.TimeStart = "14:00"
	diff = DetailedLessonChanges([]domain.Lesson{a, b}, []domain.Lesson{c, d})
	if len(diff) != 4 {
		t.Fatalf("ambiguous pairing: %+v", diff)
	}
	for _, change := range diff {
		if change.Before != nil && change.After != nil {
			t.Fatal("guessed ambiguous move")
		}
	}
}
func TestDescribeChangesFiltersPersonalScheduleAndTeacher(t *testing.T) {
	before := assistantLesson()
	after := before
	after.Room = "308"
	target := domain.PersonalTarget{ID: "group", Role: domain.RoleStudent, Timezone: "Europe/Moscow", Subgroup: 1}
	transition := domain.ScheduleTransition{Before: []domain.Lesson{before}, After: []domain.Lesson{after}}
	now := assistantDate("2026-10-04").Add(22 * time.Hour)
	text, dates, err := DescribeScheduleTransition(transition, target, nil, now)
	if err != nil || !strings.Contains(text, "Сегодня") || !strings.Contains(text, "214 → 308") || len(dates) != 1 || dates[0] != "20261005" {
		t.Fatalf("%q %v %v", text, dates, err)
	}
	transition.Before[0].Subgroup = 2
	transition.After[0].Subgroup = 2
	text, _, err = DescribeScheduleTransition(transition, target, nil, now)
	if err != nil || text != "" {
		t.Fatalf("other subgroup leaked: %q %v", text, err)
	}
	transition.Before[0].Subgroup = 0
	transition.After[0].Subgroup = 0
	changes := []domain.PersonalOverride{{LessonID: "lesson", SemesterID: "term", ValidFrom: assistantDate("2026-10-05"), ValidTo: assistantDate("2026-10-05"), Patch: json.RawMessage(`{"room":"личная"}`)}}
	text, _, err = DescribeScheduleTransition(transition, target, changes, now)
	if err != nil || text != "" {
		t.Fatalf("masked room change: %q %v", text, err)
	}
	transition.After[0].Teacher = "Петров П.П."
	target = domain.PersonalTarget{Role: domain.RoleTeacher, Name: "Сизева О. В.", Timezone: "Europe/Moscow"}
	text, _, err = DescribeScheduleTransition(transition, target, nil, now)
	if err != nil || !strings.Contains(text, "убрано") || !strings.Contains(text, "4/147") || strings.Contains(text, "добавлено") {
		t.Fatalf("teacher removal: %q %v", text, err)
	}
}
func TestNotificationPreviewUsesDeliveryWeekdaysPauseAndQuietHours(t *testing.T) {
	zone, _ := time.LoadLocation("Asia/Vladivostok")
	now := time.Date(2026, 10, 5, 19, 0, 0, 0, zone)
	published := now
	p := domain.NotificationPreferences{DailyEnabled: true, DailyTime: "20:00", DailyDayOffset: 1, DailyWeekdays: 1, DailySkipEmpty: true, ReminderEnabled: true, ReminderMinutes: 20, QuietEnabled: true, QuietStart: "19:00", QuietEnd: "23:00"}
	agenda := &PersonalAgenda{Target: domain.PersonalTarget{Timezone: zone.String()}, Freshness: &domain.SourceFreshness{LastSuccess: &published}, Days: []PersonalDay{{Date: "2026-10-06", Lessons: []PersonalLesson{{Lesson: domain.Lesson{Subject: "Math", TimeStart: "20:00", TimeEnd: "21:00"}}}}}}
	got := PreviewNotifications(p, agenda, now)
	if len(got) != 1 || got[0].Kind != "daily" || got[0].Date != "2026-10-06" || got[0].At.Hour() != 20 {
		t.Fatalf("preview: %+v", got)
	}
	p.PausedUntil = "2026-10-07"
	if got = PreviewNotifications(p, agenda, now); len(got) != 0 {
		t.Fatalf("paused: %+v", got)
	}
	p.PausedUntil = ""
	agenda.Days = nil
	if got = PreviewNotifications(p, agenda, now); len(got) != 0 {
		t.Fatalf("empty published day: %+v", got)
	}
	agenda.Freshness = nil
	if got = PreviewNotifications(p, agenda, now); len(got) != 1 {
		t.Fatalf("unpublished suppressed: %+v", got)
	}
}
func TestNoteBindingSurvivesMoveCancellationAndAmbiguity(t *testing.T) {
	lesson := assistantLesson()
	basis, _ := json.Marshal(lesson)
	note := domain.PersonalNote{LessonID: "lesson", LessonDate: *lesson.SpecialDate, Basis: basis, Body: "Отчёт", DueDate: lesson.SpecialDate}
	moved := lesson
	moved.ID = "new"
	moved.PersonalKey = "lesson"
	moved.TimeStart = "11:00"
	agenda := &PersonalAgenda{Days: []PersonalDay{{Date: "2026-10-06", Lessons: []PersonalLesson{{Original: moved, Lesson: moved}}}}}
	if got := ResolveNoteBinding(note, agenda); got.Binding != "moved" || got.Body != "Отчёт" || got.CurrentDate != "2026-10-06" {
		t.Fatalf("moved: %+v", got)
	}
	moved.PersonalKey = "unrelated"
	agenda.Days[0].Lessons[0].Original = moved
	if got := ResolveNoteBinding(note, agenda); got.Binding != "needs_review" {
		t.Fatalf("matched unrelated lecture with same subject: %+v", got)
	}
	moved.PersonalKey = "lesson"
	agenda.Days[0].Lessons[0].Original = moved
	agenda.Days[0].Lessons[0].Cancelled = true
	if got := ResolveNoteBinding(note, agenda); got.Binding != "cancelled" || got.DueDate == nil {
		t.Fatalf("cancelled: %+v", got)
	}
	agenda.Days[0].Lessons = append(agenda.Days[0].Lessons, PersonalLesson{Original: moved})
	if got := ResolveNoteBinding(note, agenda); got.Binding != "needs_review" {
		t.Fatalf("ambiguous: %+v", got)
	}
	if got := ResolveNoteBinding(note, nil); got.Binding != "needs_review" || got.Body == "" {
		t.Fatalf("removed: %+v", got)
	}
}
func TestNotificationPreferencesValidation(t *testing.T) {
	valid := domain.NotificationPreferences{DailyTime: "20:00", DailyWeekdays: 31, DailyDayOffset: 1, ReminderMinutes: 20, QuietStart: "22:00", QuietEnd: "07:00", Version: time.Now()}
	if err := ValidateNotificationPreferences(valid); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*domain.NotificationPreferences){
		func(p *domain.NotificationPreferences) { p.DailyTime = "24:00" },
		func(p *domain.NotificationPreferences) { p.DailyWeekdays = 0 },
		func(p *domain.NotificationPreferences) { p.DailyWeekdays = 128 },
		func(p *domain.NotificationPreferences) { p.DailyDayOffset = 2 },
		func(p *domain.NotificationPreferences) { p.PausedUntil = "2026-02-30" },
		func(p *domain.NotificationPreferences) { p.ReminderMinutes = 0 },
		func(p *domain.NotificationPreferences) { p.Version = time.Time{} },
	} {
		p := valid
		modify(&p)
		if err := ValidateNotificationPreferences(p); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}
