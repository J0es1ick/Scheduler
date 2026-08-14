package service

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestGeneratedLessonIdentityDoesNotDependOnSourceOrder(t *testing.T) {
	lessons := []domain.Lesson{
		{
			DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35",
			WeekType: domain.WeekTypeEvery, Subject: "Physics",
			Type: domain.LessonTypeLecture,
		},
		{
			DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35",
			WeekType: domain.WeekTypeEvery, Subject: "Mathematics",
			Type: domain.LessonTypeLecture,
		},
	}
	build := func(items []domain.Lesson) map[string]string {
		payload, _ := buildScheduleSnapshot("university", "semester", "source", []groupScheduleResult{{
			group: domain.Group{ID: "group", Name: "1/1"}, lessons: items,
		}})
		result := make(map[string]string)
		for _, lesson := range payload.Groups[0].Lessons {
			result[lesson.Subject] = lesson.ExternalID
		}
		return result
	}
	forward := build(append([]domain.Lesson(nil), lessons...))
	reverse := build([]domain.Lesson{lessons[1], lessons[0]})
	for subject, externalID := range forward {
		if reverse[subject] != externalID {
			t.Fatalf("generated identity for %s changed with source order: %q != %q",
				subject, externalID, reverse[subject])
		}
	}
}
