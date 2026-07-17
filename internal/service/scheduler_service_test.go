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

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
