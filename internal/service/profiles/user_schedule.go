package profiles

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service/validation"
)

func (s *UserService) GetTeacher(ctx context.Context, id string) (*domain.Teacher, error) {
	return s.userRepo.GetTeacher(ctx, id)
}

func (s *UserService) SetTeacher(ctx context.Context, userID, universityID, name string) error {
	return s.userRepo.SetTeacher(ctx, userID, universityID, name)
}

func (s *UserService) SetRole(ctx context.Context, userID string, role domain.UserRole) error {
	if role != domain.RoleStudent && role != domain.RoleTeacher {
		return fmt.Errorf("invalid role")
	}
	return s.userRepo.SetRole(ctx, userID, role)
}

func (s *UserService) SetTeacherView(ctx context.Context, userID string, format domain.ScheduleViewFormat) error {
	if format != domain.ScheduleViewVisual && format != domain.ScheduleViewCompact {
		return fmt.Errorf("invalid view")
	}
	return s.userRepo.SetTeacherView(ctx, userID, format)
}

func (s *UserService) SetDailySchedule(ctx context.Context, userID string, enabled bool, clock string) error {
	clock, err := validation.ParseDailyTime(clock)
	if err != nil {
		return err
	}
	return s.userRepo.SetDailySchedule(ctx, userID, enabled, clock)
}

func (s *UserService) SetDailySetup(ctx context.Context, userID, step string) error {
	if step != "choice" && step != "time" {
		return fmt.Errorf("invalid daily setup step")
	}
	return s.userRepo.SetDailySetup(ctx, userID, step)
}
