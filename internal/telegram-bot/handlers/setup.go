package handlers

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

var groupRegexp = regexp.MustCompile(`^\d+/\d+$`)

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
		prompt = "Введите номер группы (пример: 3/147):"
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

	input := c.Text()

	switch state.Step {
	case "awaiting_query":
		if !groupRegexp.MatchString(input) {
			return c.Send("Неверный формат. Пример: 3/147")
		}

		ctx, cancel := reqCtx()
		defer cancel()

		group, err := h.GroupService.FindOrCreateGroup(ctx, state.UniversityID, input, true)
		if err != nil {
			slog.Error("find or create group failed", "group", input, "err", err)
			return c.Send("Ошибка при поиске группы. Попробуйте позже.")
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
