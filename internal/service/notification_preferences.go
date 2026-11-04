package service

import (
	"context"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func ValidateNotificationPreferences(p domain.NotificationPreferences) error {
	if _, err := ParseDailyTime(p.DailyTime); err != nil {
		return personalInput(err.Error())
	}
	if p.DailyDayOffset < 0 || p.DailyDayOffset > 1 || p.DailyWeekdays < 1 || p.DailyWeekdays > 127 {
		return personalInput("Выберите день расписания и хотя бы один день отправки")
	}
	if p.ReminderMinutes < 5 || p.ReminderMinutes > 180 {
		return personalInput("Напоминание можно настроить за 5–180 минут")
	}
	for _, clock := range []string{p.QuietStart, p.QuietEnd} {
		if _, err := ParseDailyTime(clock); err != nil {
			return personalInput(err.Error())
		}
	}
	if p.QuietEnabled && p.QuietStart == p.QuietEnd {
		return personalInput("Начало и конец тихих часов должны отличаться")
	}
	if p.PausedUntil != "" {
		if _, err := time.Parse(time.DateOnly, p.PausedUntil); err != nil || len(p.PausedUntil) != 10 {
			return personalInput("Укажите дату возобновления уведомлений")
		}
	}
	if p.Version.IsZero() {
		return personalInput("Обновите настройки перед сохранением")
	}
	return nil
}

func (s *UserService) SaveNotificationPreferences(ctx context.Context, userID string, p domain.NotificationPreferences) error {
	if err := ValidateNotificationPreferences(p); err != nil {
		return err
	}
	return s.userRepo.SaveNotificationPreferences(ctx, userID, p)
}
