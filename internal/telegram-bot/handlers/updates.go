package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

type serviceUpdatesService interface {
	SetServiceUpdates(context.Context, string, bool, string) (bool, error)
	MarkServiceUpdatesPrompt(context.Context, string) error
}

func (h *Handler) showUpdatesPrompt(c tele.Context, user *domain.User) error {
	if err := c.Send("Хотите получать информацию об обновлениях сервиса?", keyboards.ServiceUpdatesPrompt(user.ServiceUpdatesPromptKey)); err != nil {
		return err
	}
	ctx, cancel := reqCtx()
	defer cancel()
	return h.UpdatesService.MarkServiceUpdatesPrompt(ctx, user.ID)
}

func (h *Handler) HandleUpdates(c tele.Context) error {
	if c.Callback() != nil {
		_ = c.Respond()
	}
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil || h.UpdatesService == nil {
		return c.Send("Сначала запустите бота: /start.")
	}
	value := strings.TrimSpace(strings.Join(c.Args(), " "))
	if c.Callback() != nil {
		args := callbackArguments(c)
		if len(args) == 1 {
			value = args[0]
		}
	}
	if value != "" {
		if value != "on" && value != "off" {
			return c.Send("Используйте /updates on или /updates off.")
		}
		if _, err = h.UpdatesService.SetServiceUpdates(ctx, user.ID, value == "on", ""); err != nil {
			return c.Send("Не удалось сохранить ответ. Попробуйте ещё раз.")
		}
		enabled := value == "on"
		user.ServiceUpdatesConsent = &enabled
	}
	status := "выключены"
	if user.ServiceUpdatesConsent != nil && *user.ServiceUpdatesConsent {
		status = "включены"
	}
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Включить", "updates_settings", "on"), menu.Data("Выключить", "updates_settings", "off")), menu.Row(menu.Data("Главное меню", "open_main_menu")))
	return c.Send("Сообщения об обновлениях сервиса: "+status+".\nЭто авторские сообщения о новых возможностях и изменениях бота. Настройки расписания от них не зависят.", menu)
}

func (h *Handler) HandleUpdatesChoice(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) != 2 || (args[0] != "on" && args[0] != "off") || h.UpdatesService == nil {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil {
		return respondStaleCallback(c)
	}
	changed, err := h.UpdatesService.SetServiceUpdates(ctx, user.ID, args[0] == "on", args[1])
	if err != nil {
		return c.Send("Не удалось сохранить ответ. Попробуйте ещё раз.")
	}
	if !changed {
		return c.Respond(&tele.CallbackResponse{Text: "Ответ уже сохранён. Изменить его можно в /updates."})
	}
	_ = c.Respond()
	text := "Рассылка обновлений выключена. Изменить решение можно в /updates."
	if args[0] == "on" {
		text = "Буду присылать сообщения об обновлениях сервиса. Отключить их можно в /updates."
	}
	if err = c.Send(text); err != nil {
		return err
	}
	if !user.ServiceUpdatesBackfill && user.DailySetup == "done" && (user.DefaultGroupID != "" || user.TeacherID != "") {
		return h.finishProfileSetup(c)
	}
	return nil
}
