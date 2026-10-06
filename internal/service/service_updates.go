package service

import "context"

func (s *UserService) SetServiceUpdates(ctx context.Context, userID string, enabled bool, promptKey string) (bool, error) {
	return s.userRepo.SetServiceUpdates(ctx, userID, enabled, promptKey)
}

func (s *UserService) MarkServiceUpdatesPrompt(ctx context.Context, userID string) error {
	return s.userRepo.MarkServiceUpdatesPrompt(ctx, userID)
}
