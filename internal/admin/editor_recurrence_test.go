package admin

import (
	"reflect"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
)

func TestOrdinaryEditPreservesOccurrenceDates(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	first := start.AddDate(0, 0, 1)
	for _, rule := range []domain.RecurrenceRule{{}, {CycleLength: 2, CycleWeeks: []int{1}, AnchorDate: &start}, {CycleLength: 3, CycleWeeks: []int{1, 3}, AnchorDate: &start}} {
		week := "odd"
		if rule.CycleLength == 3 {
			week = "every"
		}
		current := EditorLesson{SemesterID: "semester", WeekType: week, Recurrence: rule}
		before := domain.Lesson{DayOfWeek: 3, WeekType: domain.WeekType(week), ValidFrom: &first, Recurrence: rule}
		for _, explicit := range []bool{false, true} {
			mutation := LessonMutation{SemesterID: "semester", WeekType: week, Room: "New room"}
			if explicit {
				copied := rule
				mutation.Recurrence = &copied
			}
			if err := resolveRecurrence(&mutation, &current, start); err != nil {
				t.Fatal(err)
			}
			after := before
			after.Recurrence = *mutation.Recurrence
			if !reflect.DeepEqual(after.Recurrence, rule) {
				t.Fatalf("recurrence changed: %+v -> %+v", rule, after.Recurrence)
			}
			for i := 0; i < 112; i++ {
				date := start.AddDate(0, 0, i)
				if service.LessonMatchesDate(before, date, &start) != service.LessonMatchesDate(after, date, &start) {
					t.Fatalf("edit moved occurrence on %v", date)
				}
			}
		}
	}
}

func TestNewOddLessonUsesSemesterWeeks(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	lesson := LessonMutation{WeekType: "odd"}
	if err := resolveRecurrence(&lesson, nil, start); err != nil {
		t.Fatal(err)
	}
	if !lesson.Recurrence.Matches(start.AddDate(0, 0, 1), nil) || lesson.Recurrence.Matches(start.AddDate(0, 0, 8), nil) {
		t.Fatal("Wednesday parity must use Tuesday semester anchor")
	}
}
