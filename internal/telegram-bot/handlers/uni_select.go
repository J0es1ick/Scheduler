package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleUniversitySelect(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) < 2 {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Некорректный запрос"})
	}
	universityID := args[0]
	userID := c.Sender().ID
	current := h.StateManager.Get(userID)
	if !validFlow(current, "choosing_university", args[1]) {
		return respondStaleCallback(c)
	}

	ctx, cancel := reqCtx()
	defer cancel()

	selected, err := h.resolveUniversitySelection(ctx, universityID)
	if err != nil {
		slog.Error("get university failed", "id", universityID, "err", err)
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Ошибка сервера"})
	}
	if selected == nil || !selected.IsActive {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Университет недоступен"})
	}

	_ = c.Respond()

	state := current
	state.UniversityID = selected.ID
	state.University = selected.Name
	state.SearchType = dto.SearchTypeGroup
	state.Step = "awaiting_query"
	state.FlowNonce = newFlowNonce()
	state.GroupChangeDestination = "university"
	state.SetSelectedGroupDefault = true
	h.StateManager.Set(userID, state)

	return editOrSend(
		c,
		groupInputPrompt(selected.ID),
		keyboards.BackButton("back_university_selection", state.FlowNonce),
	)
}

func (h *Handler) resolveUniversitySelection(
	ctx context.Context,
	reference string,
) (*domain.University, error) {
	selectedByID, err := h.UniversityService.GetByID(ctx, reference)
	if err != nil || selectedByID != nil || !keyboards.IsUniversityToken(reference) {
		return selectedByID, err
	}
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	var selected *domain.University
	for index := range universities {
		if keyboards.UniversityToken(universities[index].ID) != reference {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("university callback token collision")
		}
		candidate := universities[index]
		selected = &candidate
	}
	return selected, nil
}
