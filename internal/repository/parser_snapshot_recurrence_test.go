package repository

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestNormalizeExternalParityRecurrence(t *testing.T) {
	start := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	payload := domain.ScheduleSnapshot{
		StartDate: start,
		Groups: []domain.SnapshotGroup{{Lessons: []domain.Lesson{
			{WeekType: domain.WeekTypeOdd},
			{WeekType: domain.WeekTypeEven},
			{WeekType: domain.WeekTypeEvery},
		}}},
	}

	result := normalizeExternalParityRecurrence(payload)
	odd := result.Groups[0].Lessons[0].Recurrence
	even := result.Groups[0].Lessons[1].Recurrence
	every := result.Groups[0].Lessons[2].Recurrence
	if odd.CycleLength != 2 || len(odd.CycleWeeks) != 1 || odd.CycleWeeks[0] != 1 ||
		odd.AnchorDate == nil || !odd.AnchorDate.Equal(start) {
		t.Fatalf("unexpected odd recurrence: %#v", odd)
	}
	if even.CycleLength != 2 || len(even.CycleWeeks) != 1 || even.CycleWeeks[0] != 2 ||
		even.AnchorDate == nil || !even.AnchorDate.Equal(start) {
		t.Fatalf("unexpected even recurrence: %#v", even)
	}
	if !every.IsZero() {
		t.Fatalf("weekly lesson recurrence was changed: %#v", every)
	}
}
