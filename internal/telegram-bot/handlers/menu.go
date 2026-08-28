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
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось загрузить профиль. Попробуйте ещё раз позже.")
	}
	if state == nil {
		state = &dto.UserState{}
	}
	state.Step = "choosing_university"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)

	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities failed", "err", err)
		return c.Send("Не удалось загрузить список вузов. Попробуйте ещё раз позже.")
	}
	_ = c.Send("Выберите новый вуз. Текущие подписки сохранятся.")
	return c.Send("Доступные вузы:", keyboards.UniversitySelector(universities, state.FlowNonce))
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
		if destination != "subscriptions" {
			return c.Send("Сначала выберите вуз: /change_university")
		}
		state = &dto.UserState{}
	}
	state.Step = "awaiting_query"
	state.SearchType = dto.SearchTypeGroup
	state.FlowNonce = newFlowNonce()
	state.GroupChangeDestination = destination
	state.GroupChangePage = page
	state.SetSelectedGroupDefault = destination != "subscriptions" || state.GroupID == ""
	h.StateManager.Set(c.Sender().ID, state)

	backArguments := []string{destination}
	if destination == "subscriptions" {
		backArguments = append(backArguments, fmt.Sprint(page))
	}
	backArguments = append(backArguments, state.FlowNonce)
	prompt := groupInputPrompt(state.UniversityID)
	if destination == "subscriptions" {
		prompt = "Добавление группы. Основная подписка не изменится.\n\n" + qualifiedGroupPrompt()
		if state.GroupID == "" {
			prompt = "Добавление первой группы. Она станет основной.\n\n" + qualifiedGroupPrompt()
		}
	} else {
		prompt = "Смена основной группы. Прежняя останется в подписках.\n\n" + prompt
	}
	markup := keyboards.BackButton("cancel_group_change", backArguments...)
	if c.Callback() != nil {
		_ = c.Respond()
		return editOrSend(c, prompt, markup)
	}
	notice, err := c.Bot().Send(c.Recipient(), "Открываю выбор группы…", &tele.ReplyMarkup{RemoveKeyboard: true})
	if err != nil {
		return err
	}
	defer func() { _ = c.Bot().Delete(notice) }()
	return c.Send(prompt, markup)
}
