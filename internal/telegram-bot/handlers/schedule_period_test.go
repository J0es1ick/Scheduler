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

func TestFormatDayScheduleUsesSafeHTMLAndTypeMarker(t *testing.T) {
	day := dto.DaySchedule{
		Date: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика <часть 1>",
			Type: domain.LessonTypeLab, Teacher: "Иванов & Петров",
		}},
	}
	formatted := formatDaySchedule(day)
	for _, expected := range []string{
		"<b>Понедельник, 10.08.2026</b>",
		"🟦 <b>08:00–09:35</b>",
		"Математика &lt;часть 1&gt;",
		"Иванов &amp; Петров",
	} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatted schedule does not contain %q: %q", expected, formatted)
		}
	}
}

func TestFormatSchedulePeriodLabelsTwoWeekRange(t *testing.T) {
	from := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	if got, want := formatSchedulePeriod(from, 14), "Период: 10.08.2026–23.08.2026"; got != want {
		t.Fatalf("formatSchedulePeriod() = %q, want %q", got, want)
	}
}

func TestScheduleWeekStartUsesMonday(t *testing.T) {
	selected := time.Date(2026, time.August, 14, 15, 30, 0, 0, time.UTC)
	want := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	if got := scheduleWeekStart(selected); !got.Equal(want) {
		t.Fatalf("scheduleWeekStart() = %s, want %s", got, want)
	}
}
