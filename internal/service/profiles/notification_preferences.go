package profiles

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service/validation"
)

func (s *UserService) SaveNotificationPreferences(ctx context.Context, userID string, p domain.NotificationPreferences) error {
	if err := validation.ValidateNotificationPreferences(p); err != nil {
		return err
	}
	return s.userRepo.SaveNotificationPreferences(ctx, userID, p)
}
