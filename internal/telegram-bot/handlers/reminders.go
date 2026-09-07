package handlers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleReminders(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil {
		slog.Error("load reminder settings failed", "user_id", userID, "err", err)
		return c.Send("Не удалось загрузить настройки напоминаний.")
	}
	if user == nil {
		return c.Send("Сначала запустите бота: /start")
	}

	input := strings.TrimSpace(strings.Join(c.Args(), " "))
	if input != "" {
		enabled, minutes, parseErr := parseReminderSetting(
			input,
			user.ReminderMinutes,
		)
		if parseErr != nil {
			return c.Send(
				"Укажите число от 5 до 180 или off.\n" +
					"Например: /reminders 45",
			)
		}
		if err = h.UserService.SetLessonReminder(
			ctx,
			userID,
			enabled,
			minutes,
		); err != nil {
			slog.Error("set lesson reminder failed", "user_id", userID, "err", err)
			return c.Send("Не удалось изменить напоминания.")
		}
		user.ReminderEnabled = enabled
		user.ReminderMinutes = minutes
	}
	return c.Send(
		reminderSettingsText(user.ReminderEnabled, user.ReminderMinutes),
		keyboards.ReminderSettings(user.ReminderEnabled, user.ReminderMinutes, 0),
	)
}

func (h *Handler) HandleShowReminderSettings(c tele.Context) error {
	_ = c.Respond()
	return h.editReminderSettings(c, callbackPage(c, 0))
}

func (h *Handler) HandleSetReminder(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) == 0 {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil || user == nil {
		return c.Send("Не удалось загрузить профиль. Используйте /start.")
	}
	enabled, minutes, err := parseReminderSetting(
		args[0],
		user.ReminderMinutes,
	)
	if err != nil {
		return c.Send("Интервал должен быть от 5 до 180 минут.")
	}
	if err = h.UserService.SetLessonReminder(
		ctx,
		userID,
		enabled,
		minutes,
	); err != nil {
		slog.Error("set lesson reminder failed", "user_id", userID, "err", err)
		return c.Send("Не удалось изменить напоминания.")
	}
	return editOrSend(
		c,
		reminderSettingsText(enabled, minutes),
		keyboards.ReminderSettings(enabled, minutes, callbackPage(c, 1)),
	)
}

func (h *Handler) HandleBackSubscriptionSettings(c tele.Context) error {
	_ = c.Respond()
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 0))
}

func (h *Handler) editReminderSettings(c tele.Context, page int) error {
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil {
		return c.Send("Не удалось загрузить профиль. Используйте /start.")
	}
	return editOrSend(
		c,
		reminderSettingsText(user.ReminderEnabled, user.ReminderMinutes),
		keyboards.ReminderSettings(user.ReminderEnabled, user.ReminderMinutes, page),
	)
}

func editOrSend(c tele.Context, text string, markup *tele.ReplyMarkup) error {
	return editOrReplace(c, text, markup)
}

func editOrReplace(c tele.Context, text string, markup *tele.ReplyMarkup, options ...interface{}) error {
	if c.Callback() == nil {
		return c.Send(text, append([]interface{}{markup}, options...)...)
	}
	if messageSupportsCaption(c.Message()) {
		if err := c.Delete(); err != nil {
			return err
		}
		return c.Send(text, append([]interface{}{markup}, options...)...)
	}
	err := c.Edit(text, append([]interface{}{markup}, options...)...)
	if err == nil || strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	if deleteErr := c.Delete(); deleteErr != nil {
		return err
	}
	return c.Send(text, append([]interface{}{markup}, options...)...)
}

func editScheduleOverlay(c tele.Context, text string, markup *tele.ReplyMarkup) error {
	if c.Callback() == nil {
		return c.Send(text, markup)
	}
	var err error
	if messageSupportsCaption(c.Message()) {
		err = c.EditCaption(text, markup)
	} else {
		err = c.Edit(text, markup)
	}
	if err == nil || strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	if deleteErr := c.Delete(); deleteErr != nil {
		return err
	}
	return c.Send(text, markup)
}

func messageSupportsCaption(message *tele.Message) bool {
	return message != nil && (message.Audio != nil || message.Document != nil ||
		message.Photo != nil || message.Video != nil || message.Animation != nil)
}

func parseReminderSetting(
	value string,
	currentMinutes int,
) (bool, int, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "off", "выкл", "выключить", "0":
		if currentMinutes < 5 || currentMinutes > 180 {
			currentMinutes = 15
		}
		return false, currentMinutes, nil
	case "on", "вкл", "включить":
		if currentMinutes < 5 || currentMinutes > 180 {
			currentMinutes = 15
		}
		return true, currentMinutes, nil
	}
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < 5 || minutes > 180 {
		return false, 0, fmt.Errorf("invalid reminder interval")
	}
	return true, minutes, nil
}

func reminderSettingsText(enabled bool, minutes int) string {
	if !enabled {
		return "Напоминания перед парами выключены.\n\n" +
			"Они приходят в личный чат для занятий основной группы. " +
			"Выберите интервал или задайте свой командой /reminders 45."
	}
	return fmt.Sprintf(
		"Напоминания включены: за %d мин. до пары основной группы.\n\n"+
			"Можно выбрать готовый интервал или задать свой: /reminders 45.",
		minutes,
	)
}
