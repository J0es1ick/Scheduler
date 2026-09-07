package service

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestReleaseCalendarCivilDatesAcrossLeapYearAndDST(t *testing.T) {
	for _, zone := range []string{"Europe/Moscow", "Europe/Berlin", "America/New_York"} {
		location, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		for _, boundary := range []struct{ anchor, first, second, third string }{
			{"2023-12-25", "2023-12-28", "2024-01-04", "2024-01-11"},
			{"2024-02-26", "2024-02-29", "2024-03-07", "2024-03-14"},
			{"2026-03-23", "2026-03-29", "2026-04-05", "2026-04-12"},
			{"2026-10-19", "2026-10-25", "2026-11-01", "2026-11-08"},
		} {
			t.Run(zone+"/"+boundary.first, func(t *testing.T) {
				anchor := mustDate(t, boundary.anchor)
				first := mustDate(t, boundary.first)
				last := mustDate(t, boundary.third)
				weekday := int(first.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				lesson := domain.Lesson{DayOfWeek: weekday, WeekType: domain.WeekTypeEvery, ValidFrom: &first, ValidTo: &last, Recurrence: domain.RecurrenceRule{CycleLength: 2, CycleWeeks: []int{1}, AnchorDate: &anchor}}
				for _, sample := range []struct {
					date string
					want bool
				}{{boundary.first, true}, {boundary.second, false}, {boundary.third, true}} {
					date, err := time.ParseInLocation(time.DateOnly, sample.date, location)
					if err != nil {
						t.Fatal(err)
					}
					if got := LessonMatchesDate(lesson, date, &anchor); got != sample.want {
						t.Fatalf("%s got=%t want=%t", sample.date, got, sample.want)
					}
				}
				if LessonMatchesDate(lesson, first.AddDate(0, 0, -14), &anchor) || LessonMatchesDate(lesson, last.AddDate(0, 0, 14), &anchor) {
					t.Fatal("escaped semester validity")
				}
				lesson.WeekType = domain.WeekTypeDate
				lesson.SpecialDate = &first
				lesson.Recurrence = domain.RecurrenceRule{}
				localFirst, _ := time.ParseInLocation(time.DateOnly, boundary.first, location)
				if !LessonMatchesDate(lesson, localFirst, &anchor) || LessonMatchesDate(lesson, localFirst.AddDate(0, 0, 1), &anchor) {
					t.Fatal("exact date drift")
				}
			})
		}
	}
}
