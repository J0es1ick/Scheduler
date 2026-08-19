package handlers

import (
	"testing"
	"time"
)

func TestFormatSchedulePeriodShowsSelectedWeekDates(t *testing.T) {
	from := time.Date(2026, time.August, 10, 15, 30, 0, 0, time.UTC)
	if got, want := formatSchedulePeriod(from, 7), "Неделя: 10.08.2026–16.08.2026"; got != want {
		t.Fatalf("formatSchedulePeriod() = %q, want %q", got, want)
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
