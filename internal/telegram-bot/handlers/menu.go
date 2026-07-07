package handlers

import (
	"log/slog"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleChange(c tgbotapi.Context) error {
	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	if state != nil {
		state.Step = "awaiting_query"
		state.SearchType = dto.SearchTypeGroup
		state.Query = ""
		state.GroupID = ""
		h.StateManager.Set(userID, state)
	}
	remove := &tgbotapi.ReplyMarkup{RemoveKeyboard: true}
	_ = c.Send("Смена группы.", remove)
	return c.Send("Введите номер группы (пример: 3/147):")
}

func (h *Handler) HandleSettings(c tgbotapi.Context) error {
	return c.Send("Настройки в разработке.\n\nЗдесь будет:\n- Уведомления об изменениях\n- Время утреннего напоминания")
}

func (h *Handler) HandleChangeUniversity(c tgbotapi.Context) error {
	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	if state != nil {
		state.Step = "awaiting_query"
		state.SearchType = dto.SearchTypeGroup
		state.Query = ""
		state.GroupID = ""
		h.StateManager.Set(userID, state)
	}

	ctx, cancel := reqCtx()
	defer cancel()

	unis, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities failed", "err", err)
		return c.Send("Ошибка загрузки университетов. Попробуйте позже.")
	}

	remove := &tgbotapi.ReplyMarkup{RemoveKeyboard: true}
	_ = c.Send("Смена параметров.", remove)
	return c.Send("Выберите новый университет:", keyboards.UniversitySelector(unis))
}

func (h *Handler) HandleChangeGroup(c tgbotapi.Context) error {
	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	if state == nil {
		return c.Send("Сначала настройте профиль: /start")
	}
	state.Step = "awaiting_query"
	state.SearchType = dto.SearchTypeGroup
	state.Query = ""
	state.GroupID = ""
	h.StateManager.Set(userID, state)

	remove := &tgbotapi.ReplyMarkup{RemoveKeyboard: true}
	_ = c.Send("Смена группы.", remove)
	return c.Send("Введите номер новой группы (пример: 3/147):")
}
