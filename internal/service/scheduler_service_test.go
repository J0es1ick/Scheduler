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
