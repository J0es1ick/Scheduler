package service

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

type ChatProfileService struct {
	repo *repository.ChatProfileRepository
}

func NewChatProfileService(repo *repository.ChatProfileRepository) *ChatProfileService {
	return &ChatProfileService{repo: repo}
}

func (s *ChatProfileService) Set(
	ctx context.Context,
	chatID string,
	title string,
	groupID string,
	configuredBy string,
) error {
	return s.repo.Upsert(ctx, chatID, title, groupID, configuredBy)
}

func (s *ChatProfileService) Get(
	ctx context.Context,
	chatID string,
) (*domain.ChatScheduleProfile, error) {
	return s.repo.Get(ctx, chatID)
}

func (s *ChatProfileService) Delete(ctx context.Context, chatID string) error {
	return s.repo.Delete(ctx, chatID)
}
