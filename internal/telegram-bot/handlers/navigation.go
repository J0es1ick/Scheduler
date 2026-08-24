package handlers

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleMenu(c tele.Context) error {
	return c.Send("Меню расписания:", keyboards.MainMenu())
}

func (h *Handler) HandleOpenMainMenu(c tele.Context) error {
	_ = c.Respond()
	return h.HandleMenu(c)
}

func (h *Handler) HandleMore(c tele.Context) error {
	return c.Send("Дополнительные разделы:", keyboards.MoreMenu())
}

func (h *Handler) HandleBackMore(c tele.Context) error {
	_ = c.Respond()
	return editOrSend(c, "Дополнительные разделы:", keyboards.MoreMenu())
}

func (h *Handler) HandleCloseInline(c tele.Context) error {
	_ = c.Respond()
	if err := c.Delete(); err != nil {
		_ = c.Edit("Меню закрыто.")
	}
	if isGroupChat(c) {
		return nil
	}
	return h.HandleMenu(c)
}

func (h *Handler) HandleOpenWeekday(c tele.Context) error {
	from := time.Now()
	if value, ok := callbackArgument(c); ok {
		parsed, err := parseScheduleDate(value, time.Local)
		if err == nil {
			from = parsed
		}
	}
	_ = c.Respond()
	return editOrSend(c, "Выберите день недели:", keyboards.WeekDaySelector(from))
}

func (h *Handler) HandleOpenScheduleGroup(c tele.Context) error {
	_ = c.Respond()
	if isGroupChat(c) {
		return h.HandleChatSettings(c)
	}
	return h.showSubscriptionSettingsPage(c, true, 0)
}

func (h *Handler) HandleAddSubscription(c tele.Context) error {
	_ = c.Respond()
	return h.beginGroupChange(c, "subscriptions", callbackPage(c, 0))
}

func (h *Handler) HandleCancelGroupChange(c tele.Context) error {
	args := callbackArguments(c)
	destination := "main"
	if len(args) > 0 && args[0] == "subscriptions" {
		destination = "subscriptions"
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile after cancelled group change failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось восстановить текущую группу. Используйте /start.")
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
		return editOrSend(c, "Настройка группы отменена. Для продолжения используйте /start.", nil)
	}
	if destination == "subscriptions" {
		return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
	}
	if err = c.Delete(); err != nil {
		_ = c.Edit("Смена группы отменена.")
	}
	return h.HandleMenu(c)
}

func (h *Handler) HandleBackUniversitySelection(c tele.Context) error {
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities for back navigation failed", "err", err)
		return c.Send("Не удалось загрузить список вузов. Попробуйте позже.")
	}
	return editOrSend(c, "Доступные вузы:", keyboards.UniversitySelector(universities))
}

func (h *Handler) HandleShowSources(c tele.Context) error {
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	text, err := h.sourcesInfoText(ctx)
	if err != nil {
		slog.Error("load public schedule sources failed", "err", err)
		return c.Send("Не удалось загрузить список источников. Попробуйте позже.")
	}
	return editOrSend(c, text, keyboards.BackToMoreMenu())
}

func (h *Handler) HandleOpenHotline(c tele.Context) error {
	_ = c.Respond()
	return h.HandleHotline(c)
}

func (h *Handler) HandleShowPrivacy(c tele.Context) error {
	_ = c.Respond()
	return editOrSend(c, privacyText(), keyboards.BackToMoreMenu())
}

func (h *Handler) HandleShowHelp(c tele.Context) error {
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	isAdmin, err := h.UserService.IsAdmin(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		slog.Debug("help role check failed", "user_id", c.Sender().ID, "err", err)
	}
	return editOrSend(c, helpText(isAdmin), keyboards.BackToMoreMenu())
}
