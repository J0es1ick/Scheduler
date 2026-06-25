// internal/service/subscription_service.go
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

// Subscribe подписывает пользователя на объект (group, teacher, room).
func (s *SubscriptionService) Subscribe(ctx context.Context, userID, objectID, objectType string) error {
	// Проверяем, нет ли уже такой подписки
	subs, err := s.subRepo.GetSubscriptionsByUserID(ctx, userID)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if sub.ObjectID == objectID && sub.ObjectType == objectType {
			return nil // уже подписан
		}
	}

	id := uuid.New().String()
	_, err = s.subRepo.CreateSubscription(ctx, id, userID, objectID, objectType)
	return err
}

// Unsubscribe отписывает пользователя от объекта.
func (s *SubscriptionService) Unsubscribe(ctx context.Context, userID, objectID, objectType string) error {
	subs, err := s.subRepo.GetSubscriptionsByUserID(ctx, userID)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if sub.ObjectID == objectID && sub.ObjectType == objectType {
			return s.subRepo.DeleteSubscription(ctx, sub.ID)
		}
	}
	return nil // не найдено – считаем успехом
}

// GetUserSubscriptions возвращает все подписки пользователя.
func (s *SubscriptionService) GetUserSubscriptions(ctx context.Context, userID string) ([]domain.Subscription, error) {
	return s.subRepo.GetSubscriptionsByUserID(ctx, userID)
}

// GetSubscribers возвращает список ID пользователей, подписанных на объект.
func (s *SubscriptionService) GetSubscribers(ctx context.Context, objectID string, objectType string) ([]string, error) {
	userIDs, err := s.subRepo.GetUserIDsByObject(ctx, objectID, objectType)
	if err != nil {
		return nil, err
	}
	return userIDs, nil
}