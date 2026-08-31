package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSearchTypeSelect(c tgbotapi.Context) error {
	userID := c.Sender().ID

	args := callbackArguments(c)
	if len(args) < 2 {
		return c.Respond(&tgbotapi.CallbackResponse{Text: "Некорректный запрос"})
	}
	searchType := dto.SearchType(args[0])

	state := h.StateManager.Get(userID)
	if !validFlow(state, "choosing_search_type", args[1]) {
		return respondStaleCallback(c)
	}

	var prompt string
	switch searchType {
	case dto.SearchTypeGroup:
		state.TeacherSearchOrigin = ""
		prompt = groupInputPrompt(state.UniversityID)
	case dto.SearchTypeTeacher:
		prompt = teacherSearchPrompt()
		state.TeacherSearchOrigin = "search"
	case dto.SearchTypeRoom:
		state.TeacherSearchOrigin = ""
		prompt = "Введите аудиторию (пример: А206):"
	case dto.SearchTypeDiscipline:
		state.TeacherSearchOrigin = ""
		prompt = "Введите дисциплину (пример: Большие данные):"
	default:
		return respondStaleCallback(c)
	}
	state.Step = "awaiting_search_query"
	state.SearchType = searchType
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(userID, state)
	_ = c.Respond()
	return editOrSend(c, prompt, keyboards.CancelButton(state.FlowNonce))
}

func (h *Handler) HandleTextInput(c tgbotapi.Context) error {
	if isGroupChat(c) {
		return nil
	}

	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	input := strings.TrimSpace(c.Text())
	if state == nil || state.Step == "done" {
		handled, err := h.handleQuickTextInput(c, input)
		if handled {
			return err
		}
	}
	if state == nil {
		return c.Send("Неизвестная команда.\n\nСписок команд: /help")
	}

	switch state.Step {
	case "awaiting_hotline_submission":
		return h.HandleHotlineSubmission(c, input)

	case "awaiting_query":
		if input == "" {
			return c.Send(qualifiedGroupPrompt(), groupInputBack(state))
		}

		ctx, cancel := reqCtx()
		defer cancel()

		universities, err := h.UniversityService.GetAll(ctx)
		if err != nil {
			return c.Send("Не удалось загрузить вузы. Попробуйте позже.", groupInputBack(state))
		}
		university, query, err := resolveGroupInput(input, state.UniversityID, state.GroupChangeDestination == "subscriptions", universities)
		if err != nil {
			return c.Send(err.Error(), groupInputBack(state))
		}
		var group *domain.Group
		variants := groupQueryVariants(query)
		for _, variant := range variants {
			group, err = h.GroupService.GetGroupByName(ctx, university.ID, variant)
			if err != nil {
				slog.Error("find group failed", "group", variant, "err", err)
				return c.Send("Ошибка при поиске группы. Попробуйте позже.", groupInputBack(state))
			}
			if group != nil {
				break
			}
		}
		if group == nil {
			groups, searchErr := h.GroupService.FindActiveByName(ctx, university.ID, variants[len(variants)-1])
			if searchErr != nil {
				return c.Send("Не удалось выполнить поиск. Попробуйте позже.", groupInputBack(state))
			}
			return c.Send(qualifiedGroupSuggestionsText(university.Name, groups), groupInputBack(state))
		}

		state.GroupID = group.ID
		state.Query = group.Name
		state.UniversityID = university.ID
		state.University = university.Name

		userIDText := fmt.Sprint(userID)
		var saveErr error
		if state.SetSelectedGroupDefault {
			saveErr = h.SubscriptionService.SubscribeAndSetDefault(ctx, userIDText, state.GroupID)
		} else {
			saveErr = h.SubscriptionService.Subscribe(ctx, userIDText, state.GroupID, "group")
		}
		if saveErr != nil {
			slog.Error("subscribe failed", "user", userID, "group", state.GroupID, "err", saveErr)
			return c.Send("Не удалось сохранить подписку. Попробуйте ещё раз позже.", groupInputBack(state))
		}
		if state.GroupChangeDestination == "subscriptions" {
			if _, _, err = h.restoreProfile(ctx, userID); err != nil {
				return c.Send("Группа добавлена, но не удалось восстановить основную группу. Откройте /settings.")
			}
			return h.showSubscriptionSettingsPage(c, false, 0)
		}

		state.Step = "done"
		state.GroupActive = group.IsActive
		h.StateManager.Set(userID, state)

		text := fmt.Sprintf(
			"Настройка завершена.\nУниверситет: %s\nГруппа: %s\n\nТеперь выберите действие:",
			state.University,
			group.Name,
		)
		return c.Send(text, keyboards.MainMenu())

	case "awaiting_search_query":
		state.SearchQuery = input
		h.StateManager.Set(userID, state)
		return h.HandleSearchResult(c, state)

	case "choosing_teacher":
		ctx, cancel := reqCtx()
		defer cancel()
		return h.beginTeacherSearch(ctx, c, state, input, state.TeacherSearchOrigin)

	default:
		return c.Send("Неизвестная команда.\n\nСписок команд: /help")
	}
}

func groupInputPrompt(universityID string) string {
	return "Введите группу выбранного вуза, например 4/147 или 4 курс 147 группа. " +
		"Можно указать другой вуз: ИГХТУ 4/147 или ИГЭУ 1-ЭЭ-В. Регистр не важен."
}

func qualifiedGroupSuggestionsText(university string, groups []domain.Group) string {
	if len(groups) == 0 {
		return university + ": группа не найдена в актуальном расписании. Проверьте название и повторите запрос с аббревиатурой вуза."
	}
	var result strings.Builder
	result.WriteString("Точного совпадения нет. Отправьте один из вариантов целиком:\n")
	for _, group := range groups {
		fmt.Fprintf(&result, "\n%s %s", university, group.Name)
	}
	return result.String()
}
