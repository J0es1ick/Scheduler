package service

import (
	"context"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func (s *ScheduleService) GetTimeSlots(ctx context.Context, universityID string, from, to time.Time) ([]domain.LessonTimeSlot, error) {
	return s.lessonRepo.GetTimeSlots(ctx, universityID, from, to)
}
