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

func personalizeReminderSchedule(ctx context.Context, provider reminderScheduleProvider, recipient domain.ReminderRecipient, date time.Time, lessons []domain.Lesson) ([]domain.Lesson, error) {
	personal, ok := provider.(interface {
		PersonalizeSchedule(context.Context, string, string, string, string, map[time.Time][]domain.Lesson) (map[time.Time][]domain.Lesson, error)
	})
	if !ok {
		return lessons, nil
	}
	teacher := ""
	if recipient.TeacherID != "" {
		teacher = recipient.GroupName
	}
	data, err := personal.PersonalizeSchedule(ctx, recipient.UserID, recipient.GroupID, recipient.UniversityID, teacher, map[time.Time][]domain.Lesson{date: lessons})
	if err != nil {
		return nil, err
	}
	return data[date], nil
}
