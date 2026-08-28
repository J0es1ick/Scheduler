package ispu

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scraper"
)

func TestPublishedUndatedLectureSchedule(t *testing.T) {
	doc := mustDocument(t, publishedLectureFixture(t))
	adapter := New("ispu-current")
	adapter.groups["101032"] = groupDescriptor{
		name:  "1-40",
		paths: []groupPath{{selectedPage: &webFormsPage{doc: doc}, scheduleLabel: "лекционное"}},
	}
	lessons, err := adapter.FetchSchedule(context.Background(), "ispu:group:101032")
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 40 {
		t.Fatalf("expected 40 lessons from 39 occupied cells (one contains two subjects), got %d", len(lessons))
	}
	ids := make(map[string]bool)
	for _, lesson := range lessons {
		if lesson.ID == "" || ids[lesson.ID] {
			t.Fatalf("missing or duplicate stable ID: %#v", lesson)
		}
		ids[lesson.ID] = true
		if lesson.ValidFrom == nil || lesson.ValidTo == nil || lesson.ValidFrom.Format("2006-01-02") < "2026-09-01" || lesson.ValidFrom.Format("2006-01-02") > "2026-09-14" || lesson.ValidTo.Format("2006-01-02") != "2026-09-14" || weekdayNumber(*lesson.ValidFrom) != lesson.DayOfWeek {
			t.Fatalf("incorrect period or weekday: %#v", lesson)
		}
		if strings.HasPrefix(lesson.Subject, "Программирование") && (lesson.Teacher != "Евсеева А. В." || !strings.HasPrefix(lesson.Room, "Б")) {
			t.Fatalf("spaced teacher initials were not recognized: %#v", lesson)
		}
	}
	for _, want := range []struct {
		date, start, end, subject, teacher, room string
		week                                     domain.WeekType
	}{
		{"2026-09-01", "08:00", "13:15", "Ознакомительная практика", "Залипаева Е.А.", "А209", domain.WeekTypeEven},
		{"2026-09-02", "09:50", "11:25", "Основы экономики", "Раева Т.Д.", "Б403", domain.WeekTypeEven},
		{"2026-09-02", "14:00", "15:35", "Ин.яз.", "Лобанова Т.Е.", "Б001в", domain.WeekTypeEven},
		{"2026-09-02", "14:00", "15:35", "Ин.яз.с \"0\"(англ.)", "Абрамова А.А.", "Б017б", domain.WeekTypeEven},
		{"2026-09-03", "11:40", "15:35", "Ознакомительная практика", "Хрипунов А.С.", "Б301", domain.WeekTypeEven},
		{"2026-09-05", "14:00", "15:35", "Ознакомительная практика", "Сухорукова Л.В.", "А330", domain.WeekTypeEven},
		{"2026-09-07", "08:00", "09:35", "Высш.матем.", "Артамонов М.А.", "Б316", domain.WeekTypeOdd},
		{"2026-09-14", "08:00", "09:35", "Дискр.матем.", "Егорова Н.Е.", "Б403", domain.WeekTypeEven},
	} {
		found := false
		for _, lesson := range lessons {
			if lesson.ValidFrom.Format("2006-01-02") == want.date && lesson.TimeStart == want.start && lesson.Subject == want.subject {
				found = true
				if lesson.TimeEnd != want.end || lesson.Teacher != want.teacher || lesson.Room != want.room || lesson.WeekType != want.week {
					t.Errorf("incorrect lesson: %#v; want %#v", lesson, want)
				}
			}
		}
		if !found {
			t.Errorf("lesson not found: %#v", want)
		}
	}
}

func TestScheduleSpanningMultipleDaysAndSlots(t *testing.T) {
	doc := mustDocument(t, weeklyTable("начало:07.09.2026 понедельник 1 недели - окончание:20.09.2026", `
		<tr><td rowspan="2">1</td><td>8.00-9.35</td><td rowspan="2" colspan="2">Физика лек. Иванов И.И. А101</td><td></td><td></td><td></td><td></td><td></td></tr>
		<tr><td>9.50-11.25</td><td style="background:#FFFFFF">Математика пр. Петров П.П. А102</td><td></td><td></td><td></td><td></td></tr>`))
	lessons, err := parseScheduleTable(doc, "ispu:group:1", "semester", "лекционное")
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 3 {
		t.Fatalf("expected three lessons, got %#v", lessons)
	}
	for i, date := range []string{"2026-09-07", "2026-09-08"} {
		if lessons[i].ValidFrom.Format("2006-01-02") != date || lessons[i].TimeStart != "08:00" || lessons[i].TimeEnd != "11:25" {
			t.Errorf("incorrect merged lesson: %#v", lessons[i])
		}
	}
	if lessons[2].Subject != "Математика" || lessons[2].DayOfWeek != 3 || lessons[2].TimeStart != "09:50" {
		t.Fatalf("lesson after rowspan shifted to another day: %#v", lessons[2])
	}
}

func TestUndatedScheduleRejectsAmbiguousOrBrokenMarkup(t *testing.T) {
	fixture := publishedLectureFixture(t)
	for name, html := range map[string]string{
		"missing anchor":       strings.ReplaceAll(fixture, "вторник 2 недели", ""),
		"missing period":       strings.ReplaceAll(fixture, "начало:", "с:"),
		"invalid period":       strings.ReplaceAll(fixture, "01.09.2026", "31.09.2026"),
		"reversed period":      strings.ReplaceAll(fixture, "14.09.2026", "31.08.2026"),
		"unknown week":         strings.Replace(fixture, `class="caption">2</td>`, `class="caption">3</td>`, 1),
		"broken time":          strings.ReplaceAll(fixture, "8.00 - 9.35", "25.00 - 26.35"),
		"changed weekdays":     strings.ReplaceAll(fixture, "понедельник</td>", "вторник</td>"),
		"oversized rowspan":    strings.ReplaceAll(fixture, `rowspan="3"`, `rowspan="999999"`),
		"lesson crosses weeks": strings.Replace(fixture, `rowspan="1" width="150px" style="background:#FFFFCC"`, `rowspan="8" width="150px" style="background:#FFFFCC"`, 1),
		"missing lesson rows":  weeklyTable("начало:01.09.2026 вторник 2 недели - окончание:14.09.2026", ""),
	} {
		t.Run(name, func(t *testing.T) {
			lessons, err := parseScheduleTable(mustDocument(t, html), "ispu:group:1", "semester", "лекционное")
			if err == nil || len(lessons) != 0 {
				t.Fatalf("broken table accepted as successful schedule: lessons=%d err=%v", len(lessons), err)
			}
			diagnostic, ok := scraper.ExtractResponseDiagnostic(err)
			if !ok || !diagnostic.StopBatch || diagnostic.Retryable || diagnostic.Category != "markup_changed" || diagnostic.ResponseSHA256 == "" || diagnostic.ResponsePreview == "" {
				t.Fatalf("missing non-retryable diagnostic: %#v", diagnostic)
			}
		})
	}
}

func TestScheduleEmptyAndOutsidePublishedPeriod(t *testing.T) {
	for name, html := range map[string]string{
		"explicit absence":              `<p>Расписание отсутствует</p>`,
		"empty status in table":         `<table id="sheduleTable"><tr><td colspan="9">Занятий нет</td></tr></table>`,
		"empty valid grid":              weeklyTable("начало:01.09.2026 вторник 2 недели - окончание:14.09.2026", `<tr><td>1</td><td>8.00-9.35</td><td></td><td></td><td></td><td></td><td></td><td></td><td></td></tr>`),
		"no occurrence in short period": weeklyTable("начало:01.09.2026 вторник 2 недели - окончание:01.09.2026", `<tr><td>1</td><td>8.00-9.35</td><td>Физика лек. Иванов И.И. А101</td><td></td><td></td><td></td><td></td><td></td><td></td></tr>`),
	} {
		t.Run(name, func(t *testing.T) {
			lessons, err := parseScheduleTable(mustDocument(t, html), "ispu:group:1", "semester", "лекционное")
			if err != nil || len(lessons) != 0 {
				t.Fatalf("expected legitimate empty schedule: lessons=%#v err=%v", lessons, err)
			}
		})
	}
}

func TestUndatedOneOffScheduleDoesNotInventRecurringDates(t *testing.T) {
	for _, end := range []string{"14.09.2026", "30.09.2026"} {
		html := weeklyTable("начало:01.09.2026 вторник 2 недели - окончание:"+end, `<tr><td>2</td><td>8.00-9.35</td><td></td><td>Физика экз. Иванов И.И. А101</td><td></td><td></td><td></td><td></td><td></td></tr>`)
		lessons, err := parseScheduleTable(mustDocument(t, html), "ispu:group:1", "semester", "экзамены")
		if end == "30.09.2026" {
			if err == nil {
				t.Fatal("ambiguous one-off date accepted")
			}
			continue
		}
		if err != nil || len(lessons) != 1 || lessons[0].SpecialDate == nil || lessons[0].SpecialDate.Format("2006-01-02") != "2026-09-01" || lessons[0].WeekType != domain.WeekTypeDate {
			t.Fatalf("incorrect one-off schedule: %#v, %v", lessons, err)
		}
	}
}

func TestDatedHeadersRemainAuthoritativeAcrossWeekBlocks(t *testing.T) {
	var header strings.Builder
	start := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	header.WriteString(`<tr><td>нед</td><td>Время</td>`)
	for day, name := range weekdayHeadings {
		fmt.Fprintf(&header, "<td>%s<br>%s</td>", name, start.AddDate(0, 0, day).Format("02.01.2006"))
	}
	header.WriteString(`</tr>`)
	body := `<tr><td>1</td><td>8.00-9.35</td><td>Физика лек.</td><td></td><td></td><td></td><td></td><td></td><td></td></tr><tr><td>2</td><td>8.00-9.35</td><td>Математика лек.</td><td></td><td></td><td></td><td></td><td></td><td></td></tr>`
	html := `<p>начало:02.02.2026 - окончание:30.06.2026</p><table id="sheduleTable">` + header.String() + body + `</table>`
	lessons, err := parseScheduleTable(mustDocument(t, html), "ispu:group:1", "semester", "лекционное")
	if err != nil || len(lessons) != 2 {
		t.Fatalf("dated table rejected: %#v, %v", lessons, err)
	}
	if lessons[0].ValidFrom.Format("2006-01-02") != "2026-02-02" || lessons[1].ValidFrom.Format("2006-01-02") != "2026-02-09" {
		t.Fatalf("week blocks received incorrect dates: %#v", lessons)
	}
}

func weeklyTable(period, rows string) string {
	return `<p>Расписание (` + period + `)</p><table id="sheduleTable"><tr><td rowspan="2">нед</td><td rowspan="2">Время</td><td colspan="7">Занятия</td></tr><tr><td>понедельник</td><td>вторник</td><td>среда</td><td>четверг</td><td>пятница</td><td>суббота</td><td>воскресенье</td></tr>` + rows + `</table>`
}

func publishedLectureFixture(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("testdata/lecture_2026_group_1_40.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
