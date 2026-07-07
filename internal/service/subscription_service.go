package service

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

type SubscriptionService struct {
	subRepo *repository.SubscriptionRepository
}

func NewSubscriptionService(subRepo *repository.SubscriptionRepository) *SubscriptionService {
	return &SubscriptionService{subRepo: subRepo}
}

func (s *SubscriptionService) Subscribe(ctx context.Context, userID, objectID, objectType string) error {
	id := uuid.New().String()
	return s.subRepo.UpsertSubscription(ctx, id, userID, objectID, objectType)
}

func (s *SubscriptionService) Unsubscribe(ctx context.Context, userID, objectID, objectType string) error {
	return s.subRepo.DeleteSubscriptionByObject(ctx, userID, objectID, objectType)
}

func (s *SubscriptionService) GetUserSubscriptions(ctx context.Context, userID string) ([]domain.Subscription, error) {
	return s.subRepo.GetSubscriptionsByUserID(ctx, userID)
}

func (s *SubscriptionService) GetSubscribers(ctx context.Context, objectID, objectType string) ([]string, error) {
	return s.subRepo.GetUserIDsByObject(ctx, objectID, objectType)
}