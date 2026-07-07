package handlers

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSearch(c tgbotapi.Context) error {
	state := h.StateManager.Get(c.Sender().ID)
	if state == nil || state.Step != "done" {
		return c.Send("Сначала настройте профиль: /start")
	}
	return c.Send("Выберите критерий поиска:", keyboards.SearchTypeSelector())
}

// HandleCancelSearch — возврат в главное меню из режима поиска.
func (h *Handler) HandleCancelSearch(c tgbotapi.Context) error {
	userID := c.Sender().ID
	state := h.StateManager.Get(userID)
	if state != nil {
		state.Step = "done"
		state.SearchQuery = ""
		h.StateManager.Set(userID, state)
	}
	_ = c.Respond()
	_ = c.Edit("Поиск отменён.")
	return c.Send("Главное меню:", keyboards.MainMenu())
}

// HandleSearchResult выполняет поиск расписания по заданному в state.SearchQuery критерию.
// Создаёт собственный контекст с таймаутом — вызывается напрямую из HandleTextInput.
func (h *Handler) HandleSearchResult(c tgbotapi.Context, state *dto.UserState) error {
	ctx, cancel := reqCtx()
	defer cancel()

	now := time.Now()
	to := now.AddDate(0, 0, 6)

	var days []dto.DaySchedule

	switch state.SearchType {
	case dto.SearchTypeGroup:
		group, err := h.GroupService.GetGroupByName(ctx, state.UniversityID, state.SearchQuery)
		if err != nil || group == nil {
			state.Step = "awaiting_search_query"
			h.StateManager.Set(c.Sender().ID, state)
			return c.Send("Группа не найдена.\nПопробуйте ввести снова:", keyboards.CancelButton())
		}
		data, err := h.ScheduleService.GetScheduleForGroupRange(ctx, group.ID, now, to)
		if err != nil {
			return c.Send("Ошибка получения расписания.")
		}
		days = mapToDaySchedule(data)

	case dto.SearchTypeTeacher:
		data, err := h.ScheduleService.GetScheduleForTeacherRange(ctx, state.SearchQuery, now, to)
		if err != nil {
			return c.Send("Ошибка получения расписания.")
		}
		days = mapToDaySchedule(data)

	case dto.SearchTypeRoom:
		data, err := h.ScheduleService.GetScheduleForRoomRange(ctx, state.SearchQuery, now, to)
		if err != nil {
			return c.Send("Ошибка получения расписания.")
		}
		days = mapToDaySchedule(data)

	case dto.SearchTypeDiscipline:
		data, err := h.ScheduleService.GetScheduleForGroupRange(ctx, state.GroupID, now, to)
		if err != nil {
			return c.Send("Ошибка получения расписания.")
		}
		for date, lessons := range data {
			var matched []domain.Lesson
			for _, l := range lessons {
				if strings.Contains(strings.ToLower(l.Subject), strings.ToLower(state.SearchQuery)) {
					matched = append(matched, l)
				}
			}
			if len(matched) > 0 {
				days = append(days, dto.DaySchedule{Date: date, Lessons: matched})
			}
		}
	}

	if len(days) == 0 {
		state.Step = "awaiting_search_query"
		h.StateManager.Set(c.Sender().ID, state)
		return c.Send(
			"По вашему запросу ничего не найдено.\nПопробуйте ввести снова или вернитесь назад:",
			keyboards.CancelButton(),
		)
	}

	state.Step = "done"
	h.StateManager.Set(c.Sender().ID, state)

	slog.Info("search completed", "type", state.SearchType, "query", state.SearchQuery, "days", len(days))

	header := fmt.Sprintf("Результаты поиска: *%s*", state.SearchQuery)
	if err := c.Send(header, &tgbotapi.SendOptions{ParseMode: tgbotapi.ModeMarkdown}); err != nil {
		return err
	}
	return h.sendDays(c, days)
}
