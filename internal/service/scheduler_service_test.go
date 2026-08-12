package service

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestLessonMatchesDateUsesSourceValidityAsParityAnchor(t *testing.T) {
	from := mustDate(t, "2026-02-10")
	to := mustDate(t, "2026-06-16")
	lesson := domain.Lesson{
		DayOfWeek: 2,
		WeekType:  domain.WeekTypeOdd,
		ValidFrom: &from,
		ValidTo:   &to,
	}

	tests := []struct {
		date string
		want bool
	}{
		{"2026-02-10", true},
		{"2026-02-17", false},
		{"2026-02-24", true},
		{"2026-06-23", false},
	}
	for _, test := range tests {
		t.Run(test.date, func(t *testing.T) {
			got := lessonMatchesDate(lesson, mustDate(t, test.date), nil)
			if got != test.want {
				t.Fatalf("lessonMatchesDate(%s) = %v, want %v", test.date, got, test.want)
			}
		})
	}
}

func TestLessonMatchesDateDistinguishesOddAndEvenByExplicitTermRecurrence(t *testing.T) {
	semesterStart := mustDate(t, "2026-08-31") // понедельник первой, нечётной недели
	validTo := mustDate(t, "2027-01-31")
	odd := domain.Lesson{
		DayOfWeek: 2,
		WeekType:  domain.WeekTypeOdd,
		ValidFrom: &semesterStart,
		ValidTo:   &validTo,
		Recurrence: domain.RecurrenceRule{
			CycleLength: 2,
			CycleWeeks:  []int{1},
			AnchorDate:  &semesterStart,
		},
	}
	even := odd
	even.WeekType = domain.WeekTypeEven
	even.Recurrence.CycleWeeks = []int{2}

	firstTuesday := mustDate(t, "2026-09-01")
	secondTuesday := mustDate(t, "2026-09-08")
	if !lessonMatchesDate(odd, firstTuesday, &semesterStart) || lessonMatchesDate(even, firstTuesday, &semesterStart) {
		t.Fatal("первая неделя должна содержать только нечётное занятие")
	}
	if lessonMatchesDate(odd, secondTuesday, &semesterStart) || !lessonMatchesDate(even, secondTuesday, &semesterStart) {
		t.Fatal("вторая неделя должна содержать только чётное занятие")
	}
}

func TestLessonMatchesDateUsesSemesterWeekWithoutSourceValidity(t *testing.T) {
	semesterStart := mustDate(t, "2026-08-31")
	lesson := domain.Lesson{DayOfWeek: 2, WeekType: domain.WeekTypeEven}
	if lessonMatchesDate(lesson, mustDate(t, "2026-09-01"), &semesterStart) {
		t.Fatal("odd semester week must not contain an even lesson")
	}
	if !lessonMatchesDate(lesson, mustDate(t, "2026-09-08"), &semesterStart) {
		t.Fatal("even semester week must contain an even lesson")
	}
}

func TestLessonMatchesDateUsesCivilDatesAcrossLocations(t *testing.T) {
	moscow := time.FixedZone("MSK", 3*60*60)
	specialDate := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	lesson := domain.Lesson{SpecialDate: &specialDate}
	queryDate := time.Date(2026, 3, 2, 12, 0, 0, 0, moscow)

	if !lessonMatchesDate(lesson, queryDate, nil) {
		t.Fatal("special date must match the same civil date in another timezone")
	}

	validFrom := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)
	oddLesson := domain.Lesson{
		DayOfWeek: 1,
		WeekType:  domain.WeekTypeOdd,
		ValidFrom: &validFrom,
	}
	twoWeeksLater := time.Date(2026, 2, 23, 8, 0, 0, 0, moscow)
	if !lessonMatchesDate(oddLesson, twoWeeksLater, nil) {
		t.Fatal("validity parity must be calculated from civil dates")
	}
}

func TestCompareLessonSnapshotsIgnoresIDsOrderAndUpdatedAt(t *testing.T) {
	first := domain.Lesson{
		ID: "source:old", GroupID: "g1", UniversityID: "u1", SemesterID: "s1",
		DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35", WeekType: domain.WeekTypeEvery,
		Subject: "Математика", Type: domain.LessonTypeLecture, UpdatedAt: time.Now(),
	}
	second := first
	second.ID = "source:new"
	second.UpdatedAt = first.UpdatedAt.Add(time.Hour)

	diff := CompareLessonSnapshots([]domain.Lesson{first}, []domain.Lesson{second})
	if diff.Changed() {
		t.Fatalf("equal schedule content reported as changed: %+v", diff)
	}
}

func TestCompareLessonSnapshotsIgnoresSourceIdentity(t *testing.T) {
	before := domain.Lesson{ID: "old", ExternalID: "source-old", SourceID: "source-a", Subject: "Math", TimeStart: "09:00", TimeEnd: "10:30"}
	after := before
	after.ID = "new"
	after.ExternalID = "source-new"
	after.SourceID = "source-b"
	if diff := CompareLessonSnapshots([]domain.Lesson{before}, []domain.Lesson{after}); diff.Changed() {
		t.Fatalf("source identity changed schedule: %+v", diff)
	}
}

func TestCompareLessonSnapshotsCountsAddedAndRemoved(t *testing.T) {
	base := domain.Lesson{
		GroupID: "g1", UniversityID: "u1", SemesterID: "s1",
		DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35", WeekType: domain.WeekTypeEvery,
		Subject: "Математика", Type: domain.LessonTypeLecture,
	}
	changed := base
	changed.Room = "А-101"

	diff := CompareLessonSnapshots([]domain.Lesson{base}, []domain.Lesson{changed})
	if diff.Added != 1 || diff.Removed != 1 {
		t.Fatalf("changed lesson diff = %+v, want one added and one removed", diff)
	}
}

func TestScheduleChangeSummaryTreatsReplacementAsModification(t *testing.T) {
	got := scheduleChangeSummary(ScheduleDiff{Added: 2, Removed: 1})
	want := "Расписание обновлено — изменено: 1, добавлено: 1."
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
