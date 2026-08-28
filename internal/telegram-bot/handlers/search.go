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
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile before search failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось загрузить профиль. Попробуйте ещё раз позже.")
	}
	if state == nil || state.Step != "done" {
		return c.Send("Сначала настройте профиль: /start")
	}
	state.Step = "choosing_search_type"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	return c.Send("Выберите критерий поиска:", keyboards.SearchTypeSelector(state.FlowNonce))
}

func (h *Handler) HandleCancelSearch(c tgbotapi.Context) error {
	userID := c.Sender().ID
	args := callbackArguments(c)
	current := h.StateManager.Get(userID)
	if len(args) < 1 || !validFlow(current, "awaiting_search_query", args[0]) {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, userID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте позже.")
	}
	if state == nil {
		return c.Send("Сначала настройте профиль: /start")
	}
	state.Step = "choosing_search_type"
	state.SearchQuery = ""
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(userID, state)
	_ = c.Respond()
	return editOrSend(c, "Выберите критерий поиска:", keyboards.SearchTypeSelector(state.FlowNonce))
}

func (h *Handler) HandleSearchResult(c tgbotapi.Context, state *dto.UserState) error {
	ctx, cancel := reqCtx()
	defer cancel()

	now := time.Now().In(h.universityLocation(ctx, state.UniversityID))
	to := now.AddDate(0, 0, 6)

	var days []dto.DaySchedule
	showGroupNames := false

	switch state.SearchType {
	case dto.SearchTypeGroup:
		universities, err := h.UniversityService.GetAll(ctx)
		if err != nil {
			return c.Send("Не удалось загрузить вузы.", keyboards.CancelButton(state.FlowNonce))
		}
		university, query, err := resolveGroupInput(state.SearchQuery, state.UniversityID, false, universities)
		if err != nil {
			return c.Send(err.Error(), keyboards.CancelButton(state.FlowNonce))
		}
		var group *domain.Group
		variants := groupQueryVariants(query)
		for _, variant := range variants {
			group, err = h.GroupService.GetGroupByName(ctx, university.ID, variant)
			if err != nil {
				return c.Send("Не удалось выполнить поиск.", keyboards.CancelButton(state.FlowNonce))
			}
			if group != nil {
				break
			}
		}
		if group == nil {
			groups, searchErr := h.GroupService.FindActiveByName(ctx, university.ID, variants[len(variants)-1])
			if searchErr != nil {
				return c.Send("Не удалось выполнить поиск. Попробуйте позже.", keyboards.CancelButton(state.FlowNonce))
			}
			return c.Send(qualifiedGroupSuggestionsText(university.Name, groups), keyboards.CancelButton(state.FlowNonce))
		}
		target := &scheduleTarget{
			GroupID: group.ID, GroupName: group.Name, UniversityID: university.ID, University: university.Name,
			ViewFormat: domain.ScheduleViewVisual, Public: true,
		}
		subscriptions, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
		if err != nil {
			return c.Send("Не удалось загрузить настройки расписания.", keyboards.CancelButton(state.FlowNonce))
		}
		for _, subscription := range subscriptions {
			if subscription.GroupID == group.ID {
				target.ViewFormat, target.Subgroup, target.Public = subscription.ScheduleViewFormat, subscription.Subgroup, false
				break
			}
		}
		state.Step = "done"
		h.StateManager.Set(c.Sender().ID, state)
		return h.sendTargetWeek(ctx, c, target, h.targetNow(ctx, target), 7)

	case dto.SearchTypeTeacher:
		showGroupNames = true
		data, err := h.ScheduleService.GetScheduleForTeacherRange(ctx, state.UniversityID, state.SearchQuery, now, to)
		if err != nil {
			return c.Send("Ошибка получения расписания.")
		}
		days = mapToDaySchedule(data)

	case dto.SearchTypeRoom:
		showGroupNames = true
		data, err := h.ScheduleService.GetScheduleForRoomRange(ctx, state.UniversityID, state.SearchQuery, now, to)
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
			keyboards.CancelButton(state.FlowNonce),
		)
	}

	state.Step = "done"
	h.StateManager.Set(c.Sender().ID, state)

	slog.Info("search completed", "type", state.SearchType, "days", len(days))

	header := fmt.Sprintf("Результаты поиска: %s", state.SearchQuery)
	if err := c.Send(header); err != nil {
		return err
	}
	if showGroupNames {
		return h.sendDaysWithGroupNames(c, days, state.UniversityID)
	}
	return h.sendDays(c, days, state.UniversityID)
}
