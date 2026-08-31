package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func TestFormatSchedulePeriodShowsSelectedWeekDates(t *testing.T) {
	from := time.Date(2026, time.August, 10, 15, 30, 0, 0, time.UTC)
	if got, want := formatSchedulePeriod(from, 7), "Неделя: 10.08.2026–16.08.2026"; got != want {
		t.Fatalf("formatSchedulePeriod() = %q, want %q", got, want)
	}
}

func TestFormatSchedulePeriodShowsSingleDate(t *testing.T) {
	from := time.Date(2026, time.August, 10, 15, 30, 0, 0, time.UTC)
	if got, want := formatSchedulePeriod(from, 1), "Дата: 10.08.2026"; got != want {
		t.Fatalf("formatSchedulePeriod() = %q, want %q", got, want)
	}
}

func TestFormatDayScheduleShowsGroupOnlyWhenRequested(t *testing.T) {
	day := dto.DaySchedule{
		Date: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика",
			Type: domain.LessonTypeLecture, Teacher: "Иванов И.И.", Room: "А-101",
			GroupName: "3/42",
		}},
	}

	if got := formatDaySchedule(day); strings.Contains(got, "группа 3/42") {
		t.Fatalf("ordinary group schedule unexpectedly contains group name: %q", got)
	}
	if got := formatDayScheduleWithGroupNames(day); !strings.Contains(got, "группа 3/42 · Иванов И.И. · А-101") {
		t.Fatalf("cross-group search result does not identify the group: %q", got)
	}
}

func TestFormatDayScheduleUsesSafeHTMLWithoutDecorativeMarkers(t *testing.T) {
	day := dto.DaySchedule{
		Date: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика <часть 1>",
			Type: domain.LessonTypeLab, Teacher: "Иванов & Петров",
		}},
	}
	formatted := formatDaySchedule(day)
	for _, expected := range []string{
		"<i>Понедельник, 10.08.2026</i>",
		"08:00–09:35 · <b>Математика &lt;часть 1&gt;</b>",
		"Иванов &amp; Петров",
	} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatted schedule does not contain %q: %q", expected, formatted)
		}
	}
	for _, marker := range []string{"🟩", "🩷", "🟦", "🟨", "🟥", "🟪", "⬜"} {
		if strings.Contains(formatted, marker) {
			t.Fatalf("formatted schedule contains decorative marker %q: %q", marker, formatted)
		}
	}
}

func TestFormatSchedulePeriodLabelsTwoWeekRange(t *testing.T) {
	from := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	if got, want := formatSchedulePeriod(from, 14), "Две недели: 10.08.2026–23.08.2026"; got != want {
		t.Fatalf("formatSchedulePeriod() = %q, want %q", got, want)
	}
}

func TestTwoWeekTextSeparatesWeeks(t *testing.T) {
	from := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	days := []dto.DaySchedule{
		{Date: from.AddDate(0, 0, 1), Lessons: []domain.Lesson{{Subject: "Первая", TimeStart: "09:50", TimeEnd: "11:25"}}},
		{Date: from.AddDate(0, 0, 7), Lessons: []domain.Lesson{{Subject: "Вторая", TimeStart: "09:50", TimeEnd: "11:25"}}},
	}
	formatted := formatScheduleDays(days, false, from, 14)
	if strings.Count(formatted, "<b>Первая неделя</b>") != 1 ||
		strings.Count(formatted, "<b>Вторая неделя</b>") != 1 {
		t.Fatalf("two-week headings missing: %q", formatted)
	}
	if strings.Index(formatted, "<b>Первая неделя</b>") > strings.Index(formatted, "Вторник, 01.09.2026") ||
		strings.Index(formatted, "<b>Вторая неделя</b>") > strings.Index(formatted, "Понедельник, 07.09.2026") {
		t.Fatalf("two-week headings are misplaced: %q", formatted)
	}
}

func TestTwoWeekTextMarksEmptyWeek(t *testing.T) {
	from := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	days := []dto.DaySchedule{{
		Date:    from.AddDate(0, 0, 8),
		Lessons: []domain.Lesson{{Subject: "Только вторая", TimeStart: "09:50", TimeEnd: "11:25"}},
	}}
	formatted := formatScheduleDays(days, false, from, 14)
	firstWeek := formatted[:strings.Index(formatted, "<b>Вторая неделя</b>")]
	if !strings.Contains(firstWeek, "Занятий нет.") {
		t.Fatalf("empty first week is not marked: %q", formatted)
	}
}

func TestScheduleWeekStartUsesMonday(t *testing.T) {
	selected := time.Date(2026, time.August, 14, 15, 30, 0, 0, time.UTC)
	want := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	if got := scheduleWeekStart(selected); !got.Equal(want) {
		t.Fatalf("scheduleWeekStart() = %s, want %s", got, want)
	}
}
