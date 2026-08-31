package handlers

import (
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSearchViewSettings(c tele.Context) error {
	if h.privateTransientFlow(c) {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Не удалось загрузить настройку", ShowAlert: true})
	}
	_ = c.Respond()
	return editOrSend(
		c,
		"Формат расписания из поиска\n\nНастройка применяется к расписанию преподавателя, открытому через поиск. По умолчанию используется визуальная таблица.",
		keyboards.SearchScheduleViewSettings(searchScheduleView(user.SearchScheduleView)),
	)
}

func (h *Handler) HandleSetSearchView(c tele.Context) error {
	value, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	format := domain.ScheduleViewFormat(value)
	if format != domain.ScheduleViewCompact && format != domain.ScheduleViewVisual {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	if err := h.UserService.SetSearchScheduleView(ctx, fmt.Sprint(c.Sender().ID), format); err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Не удалось сохранить формат", ShowAlert: true})
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "Формат сохранён"})
	return editOrSend(
		c,
		"Формат расписания из поиска сохранён.",
		keyboards.SearchScheduleViewSettings(format),
	)
}

func searchScheduleView(format domain.ScheduleViewFormat) domain.ScheduleViewFormat {
	if format == domain.ScheduleViewCompact {
		return format
	}
	return domain.ScheduleViewVisual
}
