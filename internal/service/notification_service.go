// internal/service/notification_service.go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	tgbotapi "gopkg.in/telebot.v3"
)

type NotificationService struct {
	subRepo  *repository.SubscriptionRepository
	userRepo *repository.UserRepository
	bot      *tgbotapi.Bot
}

func NewNotificationService(
	subRepo *repository.SubscriptionRepository,
	userRepo *repository.UserRepository,
	bot *tgbotapi.Bot,
) *NotificationService {
	return &NotificationService{
		subRepo:  subRepo,
		userRepo: userRepo,
		bot:      bot,
	}
}

// NotifyObjectChanged отправляет сообщение всем подписчикам на объект (группа/преподаватель/аудитория).
func (s *NotificationService) NotifyObjectChanged(ctx context.Context, objectType, objectID, message string) error {
	allSubs, err := s.subRepo.GetUserIDsByObject(ctx, objectType, objectID) 
	if err != nil {
		return err
	}
	
	for _, userID := range allSubs {
		_, err := s.bot.SendMessage(&tgbotapi.SendMessageParams{
			ChatID: userID,
			Text:   fmt.Sprintf("Обновление для %s %s: %s", objectType, objectID, message),
		})
		if err != nil {
			// Логируем ошибку, но продолжаем отправлять другим пользователям
			fmt.Printf("Failed to send notification to user %s: %v\n", userID, err)
		}
	}
	return nil
}

// SendDailyDigest отправляет каждому пользователю его подписки (например, расписание на сегодня).
// Для каждого пользователя собираем расписание по всем его подпискам.
func (s *NotificationService) SendDailyDigest(ctx context.Context, scheduleSvc *ScheduleService) error {
	users, err := s.userRepo.GetAllUsers(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		subs, err := s.subRepo.GetSubscriptionsByUserID(ctx, user.ID)
		if err != nil {
			continue
		}
		// Для простоты – собираем расписание только по группам
		var allLessons []domain.Lesson
		for _, sub := range subs {
			if sub.ObjectType == "group" {
				lessons, err := scheduleSvc.GetScheduleForGroup(ctx, sub.ObjectID, time.Now())
				if err == nil {
					allLessons = append(allLessons, lessons...)
				}
			}
		}
		if len(allLessons) > 0 {
			text := formatLessons(allLessons) // ваша функция форматирования
			s.bot.SendMessage(&tgbotapi.SendMessageParams{
				ChatID: user.ID,
				Text:   text,
			})
		}
	}
	return nil
}

func formatLessons(lessons []domain.Lesson) string {
	// реализуй вывод
	return "Ваше расписание на сегодня: ..."
}