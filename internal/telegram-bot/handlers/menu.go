package handlers

import (
	"fmt"
	"log/slog"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleChange(c tele.Context) error {
	return h.HandleChangeGroup(c)
}

func (h *Handler) HandleChangeUniversity(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()

	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities failed", "err", err)
		return c.Send("Не удалось загрузить список вузов. Попробуйте ещё раз позже.")
	}
	_ = c.Send("Выберите новый вуз. Текущие подписки сохранятся.")
	return c.Send("Доступные вузы:", keyboards.UniversitySelector(universities))
}

func (h *Handler) HandleChangeGroup(c tele.Context) error {
	return h.beginGroupChange(c, "main", 0)
}

func (h *Handler) beginGroupChange(c tele.Context, destination string, page int) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile before group change failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось загрузить профиль. Попробуйте ещё раз позже.")
	}
	if state == nil {
		return c.Send("Сначала выберите вуз: /change_university")
	}
	state.Step = "awaiting_query"
	state.SearchType = dto.SearchTypeGroup
	h.StateManager.Set(c.Sender().ID, state)

	remove := &tele.ReplyMarkup{RemoveKeyboard: true}
	_ = c.Send("Введите новую основную группу. Прежняя останется в подписках.", remove)
	backArguments := []string{destination}
	if destination == "subscriptions" {
		backArguments = append(backArguments, fmt.Sprint(page))
	}
	return c.Send(
		groupInputPrompt(state.UniversityID),
		keyboards.BackButton("cancel_group_change", backArguments...),
	)
}
