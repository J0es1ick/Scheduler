package handlers

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleStart(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()

	telegramID := fmt.Sprint(c.Sender().ID)
	username := c.Sender().Username

	user, err := h.UserService.RegisterOrGetUser(ctx, telegramID, username)
	if err != nil {
		slog.Error("user register failed", "telegramID", telegramID, "err", err)
		return c.Send("Ошибка при создании пользователя. Попробуйте позже.")
	}
	slog.Info("user started bot", "id", user.ID, "username", user.Username)

	name := c.Sender().FirstName
	if name == "" {
		name = "друг"
	}

	text := fmt.Sprintf(
		"%s, %s.\n\nДанный бот предоставляет актуальное расписание занятий.\nПолный список команд: /help",
		timeGreeting(),
		name,
	)
	if err := c.Send(text); err != nil {
		return err
	}

	unis, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities failed", "err", err)
		return c.Send("Ошибка загрузки университетов. Попробуйте позже.")
	}
	if len(unis) == 0 {
		return c.Send("Университеты пока не добавлены в систему.")
	}

	return c.Send("Выберите ваш университет:", keyboards.UniversitySelector(unis))
}

func timeGreeting() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 6 && hour < 12:
		return "Доброе утро"
	case hour >= 12 && hour < 18:
		return "Добрый день"
	case hour >= 18 && hour < 23:
		return "Добрый вечер"
	default:
		return "Доброй ночи"
	}
}
