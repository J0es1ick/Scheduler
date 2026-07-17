package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSearchTypeSelect(c tgbotapi.Context) error {
	userID := c.Sender().ID

	args := c.Args()
	if len(args) == 0 {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Некорректный запрос"})
	}
	searchType := dto.SearchType(args[0])

	state := h.StateManager.Get(userID)
	if state == nil {
		return c.Send("Произошла ошибка. Начните сначала: /start")
	}

	state.Step = "awaiting_search_query"
	state.SearchType = searchType
	h.StateManager.Set(userID, state)

	_ = c.Respond()

	var prompt string
	switch searchType {
	case dto.SearchTypeGroup:
		prompt = groupInputPrompt(state.UniversityID)
	case dto.SearchTypeTeacher:
		prompt = "Введите преподавателя (пример: Сизова О.В.):"
	case dto.SearchTypeRoom:
		prompt = "Введите аудиторию (пример: А206):"
	case dto.SearchTypeDiscipline:
		prompt = "Введите дисциплину (пример: Большие данные):"
	default:
		return c.Send("Неизвестный тип поиска.")
	}

	_ = c.Edit("Введите запрос:")
	return c.Send(prompt, keyboards.CancelButton())
}

func (h *Handler) HandleTextInput(c tgbotapi.Context) error {
	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	if state == nil {
		return c.Send("Неизвестная команда.\n\nСписок команд: /help")
	}

	input := strings.TrimSpace(c.Text())

	switch state.Step {
	case "awaiting_query":
		if input == "" {
			return c.Send(groupInputPrompt(state.UniversityID))
		}

		ctx, cancel := reqCtx()
		defer cancel()

		group, err := h.GroupService.GetGroupByName(ctx, state.UniversityID, input)
		if err != nil {
			slog.Error("find group failed", "group", input, "err", err)
			return c.Send("Ошибка при поиске группы. Попробуйте позже.")
		}
		if group == nil {
			return c.Send("Такой группы нет в актуальном расписании выбранного университета. Проверьте номер или дождитесь завершения первого обновления данных.")
		}

		state.GroupID = group.ID
		state.Query = input

		// Подписка — некритичная операция, ошибку только логируем.
		if err := h.SubscriptionService.Subscribe(ctx, fmt.Sprint(userID), state.GroupID, "group"); err != nil {
			slog.Warn("subscribe failed", "user", userID, "group", state.GroupID, "err", err)
		}

		state.Step = "done"
		h.StateManager.Set(userID, state)

		text := fmt.Sprintf(
			"Настройка завершена.\nУниверситет: %s\nГруппа: %s\n\nТеперь выберите действие:",
			state.University,
			input,
		)
		return c.Send(text, keyboards.MainMenu())

	case "awaiting_search_query":
		state.SearchQuery = input
		h.StateManager.Set(userID, state)
		return h.HandleSearchResult(c, state)

	default:
		return c.Send("Неизвестная команда.\n\nСписок команд: /help")
	}
}

func groupInputPrompt(universityID string) string {
	if universityID == "ispu" {
		return "Введите группу ИГЭУ в формате «курс-номер». Например: 1-40, 1-10м или 2-10."
	}
	return "Введите группу ИГХТУ в формате с сайта. Например: 3/147."
}
