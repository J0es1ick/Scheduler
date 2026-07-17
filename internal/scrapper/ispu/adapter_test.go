package ispu

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/PuerkitoBio/goquery"
)

func TestParseOneOffSchedule(t *testing.T) {
	doc := mustDocument(t, `
		<html><body>
		<div>Расписание экзаменов (начало:03.06.2026 - окончание: 09.06.2026)</div>
		<table id="sheduleTable">
		<tr><td>нед</td><td>Время</td><td>понедельник<br/>01.06.2026</td><td>вторник<br/>02.06.2026</td><td>среда<br/>03.06.2026</td><td>четверг<br/>04.06.2026</td><td>пятница<br/>05.06.2026</td><td>суббота<br/>06.06.2026</td><td>воскресенье<br/>07.06.2026</td></tr>
		<tr><td rowspan="7">1</td><td>8.00 - 9.35</td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#A3C3FF">История России экз. Хрипунов А.С. А402</td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#D1E1FF">Физика зач. Иванова В.Г. В310</td></tr>
		</table></body></html>`)

	lessons, err := parseScheduleTable(doc, "ispu:group:918", "ispu-current", "экзаменов и зачетов")
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 2 {
		t.Fatalf("expected 2 lessons, got %d: %#v", len(lessons), lessons)
	}
	first := lessons[0]
	if first.WeekType != domain.WeekTypeDate || first.DayOfWeek != 0 || first.SpecialDate == nil {
		t.Fatalf("unexpected one-off shape: %#v", first)
	}
	if got := first.SpecialDate.Format("2006-01-02"); got != "2026-06-03" {
		t.Fatalf("unexpected special date: %s", got)
	}
	if first.Type != domain.LessonTypeExam || first.Subject != "История России" || first.Teacher != "Хрипунов А.С." || first.Room != "А402" {
		t.Fatalf("unexpected parsed exam: %#v", first)
	}
	if lessons[1].Type != domain.LessonTypeCredit || lessons[1].SpecialDate.Format("2006-01-02") != "2026-06-07" {
		t.Fatalf("unexpected parsed credit: %#v", lessons[1])
	}
}

func TestParseRecurringSundaySchedule(t *testing.T) {
	doc := mustDocument(t, `
		<html><body>
		<div>Расписание занятий (начало:04.02.2026 - окончание: 30.06.2026)</div>
		<table id="sheduleTable">
		<tr><td>нед</td><td>Время</td><td>понедельник<br/>02.02.2026</td><td>вторник<br/>03.02.2026</td><td>среда<br/>04.02.2026</td><td>четверг<br/>05.02.2026</td><td>пятница<br/>06.02.2026</td><td>суббота<br/>07.02.2026</td><td>воскресенье<br/>08.02.2026</td></tr>
		<tr><td rowspan="7">1</td><td>9.50-11.25</td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#FFFFFF"></td><td style="background:#D1E1FF">Электротехника лек. Петров П.П. А101</td></tr>
		</table></body></html>`)

	lessons, err := parseScheduleTable(doc, "ispu:group:1", "ispu-current", "занятий")
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 1 {
		t.Fatalf("expected one lesson, got %d", len(lessons))
	}
	lesson := lessons[0]
	if lesson.DayOfWeek != 7 || lesson.WeekType != domain.WeekTypeOdd || lesson.Type != domain.LessonTypeLecture {
		t.Fatalf("unexpected recurring lesson: %#v", lesson)
	}
	if lesson.TimeStart != "09:50" || lesson.TimeEnd != "11:25" {
		t.Fatalf("unexpected time: %s-%s", lesson.TimeStart, lesson.TimeEnd)
	}
}

func TestMergeSubgroupPages(t *testing.T) {
	common := domain.Lesson{GroupID: "g", Subject: "Общая лекция", TimeStart: "08:00"}
	firstOnly := domain.Lesson{GroupID: "g", Subject: "Лабораторная А", TimeStart: "09:50"}
	secondOnly := domain.Lesson{GroupID: "g", Subject: "Лабораторная Б", TimeStart: "09:50"}

	merged := mergeSubgroupPages([][]domain.Lesson{{common, firstOnly}, {common, secondOnly}})
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged lessons, got %d", len(merged))
	}
	bySubject := make(map[string]int)
	for _, lesson := range merged {
		bySubject[lesson.Subject] = lesson.Subgroup
	}
	if bySubject["Общая лекция"] != 0 || bySubject["Лабораторная А"] != 1 || bySubject["Лабораторная Б"] != 2 {
		t.Fatalf("unexpected subgroup merge: %#v", bySubject)
	}
}

func TestLiveISPUAdapter(t *testing.T) {
	if os.Getenv("ISPU_INTEGRATION_TEST") != "1" {
		t.Skip("set ISPU_INTEGRATION_TEST=1 to query schedule.ispu.ru")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	adapter := New("ispu-current")
	groups, err := adapter.FetchGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 {
		t.Fatal("live site returned no groups")
	}
	sample := groups[0]
	for _, group := range groups {
		if group.Name == "1-40" {
			sample = group
			break
		}
	}
	lessons, err := adapter.FetchSchedule(ctx, sample.ID)
	if err != nil {
		t.Fatal(err)
	}
	subgroups := make(map[int]int)
	for _, lesson := range lessons {
		subgroups[lesson.Subgroup]++
	}
	t.Logf("groups=%d sample=%s lessons=%d subgroups=%v", len(groups), sample.Name, len(lessons), subgroups)
}

func mustDocument(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
