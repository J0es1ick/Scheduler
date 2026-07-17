package handlers

import (
	"log/slog"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleUniversitySelect(c tgbotapi.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Некорректный запрос"})
	}
	universityID := args[0]
	userID := c.Sender().ID

	ctx, cancel := reqCtx()
	defer cancel()

	selected, err := h.UniversityService.GetByID(ctx, universityID)
	if err != nil {
		slog.Error("get university failed", "id", universityID, "err", err)
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Ошибка сервера"})
	}
	if selected == nil {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Университет не найден"})
	}

	_ = c.Respond()

	state := &dto.UserState{
		UniversityID: selected.ID,
		University:   selected.Name,
		SearchType:   dto.SearchTypeGroup,
		Step:         "awaiting_query",
	}
	h.StateManager.Set(userID, state)

	return c.Edit(groupInputPrompt(selected.ID))
}
