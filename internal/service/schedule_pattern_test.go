package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestSchedulePatternFromPublishedDates(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	semester := domain.Semester{ID: "term", StartDate: start, EndDate: start.AddDate(0, 0, 7*16-1)}
	for _, test := range []struct {
		name          string
		period, weeks int
		want          string
	}{
		{"weekly dates", 1, 16, "weekly"}, {"biweekly dates", 2, 16, "biweekly"}, {"individual", 16, 16, "individual"}, {"only two weeks", 2, 2, "individual"}, {"four weekly copies", 1, 4, "weekly"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lessons := []domain.Lesson{}
			for week := 0; week < test.weeks; week++ {
				date := start.AddDate(0, 0, week*7)
				lessons = append(lessons, domain.Lesson{ID: fmt.Sprint(week), GroupID: "g", SemesterID: "term", SpecialDate: &date, WeekType: domain.WeekTypeDate, TimeStart: "09:00", TimeEnd: "10:30", Subject: fmt.Sprint(week % test.period)})
			}
			pattern := AnalyzeSchedulePattern(lessons, semester)
			if pattern.Kind != test.want {
				t.Fatalf("pattern: %+v", pattern)
			}
			if test.want != "individual" {
				repeats := pattern.Repeats(lessons[0], start)
				if len(repeats) != test.weeks/test.period {
					t.Fatalf("wrong repeats: %+v", repeats)
				}
			}
		})
	}
}

func TestSchedulePatternBoundsAndExceptions(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	semester := domain.Semester{ID: "term", StartDate: start, EndDate: end}
	first := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	lesson := domain.Lesson{ID: "l", GroupID: "g", SemesterID: "term", DayOfWeek: 2, TimeStart: "09:00", WeekType: domain.WeekTypeOdd, ValidFrom: &first, ValidTo: &last}
	pattern := AnalyzeSchedulePattern([]domain.Lesson{lesson}, semester)
	if pattern.Kind != "biweekly" {
		t.Fatalf("pattern: %+v", pattern)
	}
	for _, repeat := range pattern.Repeats(lesson, first) {
		if repeat.Date > last.Format(time.DateOnly) {
			t.Fatal("invented occurrence", repeat)
		}
	}
	exception := lesson
	exception.ID = "exception"
	exception.Subject = "exam"
	exception.SpecialDate = &last
	pattern = AnalyzeSchedulePattern([]domain.Lesson{lesson, exception}, semester)
	if len(pattern.Repeats(exception, last)) != 0 {
		t.Fatal("exception treated as recurring")
	}
	lesson.Recurrence = domain.RecurrenceRule{CycleLength: 3, CycleWeeks: []int{1}, AnchorDate: &first}
	pattern = AnalyzeSchedulePattern([]domain.Lesson{lesson}, semester)
	if pattern.Kind != "individual" {
		t.Fatal("three-week cycle misclassified", pattern)
	}
}

func TestPersonalRemapRequiresOneSlotAndPreservesSelectedDates(t *testing.T) {
	date := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	basis := domain.Lesson{ID: "old", GroupID: "g", SemesterID: "s", DayOfWeek: 1, TimeStart: "09:00", TimeEnd: "10:30", WeekType: domain.WeekTypeEvery, Subject: "Old"}
	raw, _ := json.Marshal(basis)
	change := domain.PersonalOverride{Basis: raw, Occurrences: []byte(`[{"lesson_id":"old","date":"2026-10-05"},{"lesson_id":"old","date":"2026-10-19"}]`), NeedsReview: true}
	semester := domain.Semester{StartDate: date, EndDate: date.AddDate(0, 3, 0)}
	current := basis
	current.ID = "new"
	current.Subject = "New subject"
	got, counts, err := remapPersonalChange(change, []domain.Lesson{current}, semester)
	if err != nil || counts.Kept != 2 || counts.Dropped != 0 {
		t.Fatalf("remap: %+v %v", counts, err)
	}
	got.NeedsReview = false
	got.SemesterID = "s"
	if !got.Matches(current, date) || got.Matches(current, date.AddDate(0, 0, 7)) {
		t.Fatal("selected dates were changed")
	}
	for _, variant := range []domain.Lesson{
		{GroupID: "other", SemesterID: "s", DayOfWeek: 1, TimeStart: "09:00", TimeEnd: "10:30", WeekType: domain.WeekTypeEvery},
		{GroupID: "g", SemesterID: "s", DayOfWeek: 1, TimeStart: "09:00", TimeEnd: "10:30", Subgroup: 1, WeekType: domain.WeekTypeEvery},
	} {
		_, counts, err = remapPersonalChange(change, []domain.Lesson{variant}, semester)
		if err != nil || counts.Kept != 0 || counts.Dropped != 2 {
			t.Fatal("wrong slot kept", counts, err)
		}
	}
	_, counts, err = remapPersonalChange(change, []domain.Lesson{current, current}, semester)
	if err != nil || counts.Kept != 0 {
		t.Fatal("ambiguous slot kept", counts, err)
	}
}

func TestSchedulePatternKeepsKnownRepeatsAroundExceptionalWeeks(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	semester := domain.Semester{ID: "term", StartDate: start, EndDate: start.AddDate(0, 0, 16*7-1)}
	lessons := []domain.Lesson{}
	for week := 0; week < 16; week++ {
		date := start.AddDate(0, 0, week*7)
		lesson := domain.Lesson{ID: fmt.Sprint(week), GroupID: "g", SemesterID: "term", SpecialDate: &date, Subject: fmt.Sprint(week % 2), TimeStart: "09:00", TimeEnd: "10:30"}
		lessons = append(lessons, lesson)
		if week == 8 {
			lesson.ID = "exception"
			lesson.TimeStart = "12:00"
			lesson.Subject = "Exam"
			lessons = append(lessons, lesson)
		}
	}
	pattern := AnalyzeSchedulePattern(lessons, semester)
	if pattern.Kind != "biweekly" || pattern.Exceptions != 1 || len(pattern.Repeats(lessons[0], start)) != 8 {
		t.Fatal("exception split confirmed repeats", pattern)
	}
}
