package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func reminderTarget(recipient domain.ReminderRecipient) string {
	if recipient.TeacherID != "" {
		return "teacher:" + recipient.TeacherID
	}
	return recipient.GroupID
}

func loadReminderSchedule(ctx context.Context, provider reminderScheduleProvider, recipient domain.ReminderRecipient, date time.Time) ([]domain.Lesson, error) {
	if recipient.TeacherID == "" {
		return provider.GetScheduleForGroup(ctx, recipient.GroupID, date)
	}
	teachers, ok := provider.(interface {
		GetScheduleForTeacher(context.Context, string, string, time.Time) ([]domain.Lesson, error)
	})
	if !ok {
		return nil, fmt.Errorf("teacher schedule provider unavailable")
	}
	return teachers.GetScheduleForTeacher(ctx, recipient.UniversityID, recipient.GroupName, date)
}
