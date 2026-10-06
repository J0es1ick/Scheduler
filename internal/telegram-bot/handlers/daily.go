package handlers

import (
	"fmt"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) finishProfileSetup(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, user, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil || state == nil {
		return c.Send("Профиль сохранён. Для продолжения используйте /start.")
	}
	if user.DailySetup == "choice" || user.DailySetup == "time" {
		return h.showDailySetup(c, user.DailySetup, true)
	}
	if h.UpdatesService != nil && user.ServiceUpdatesConsent == nil && !user.ServiceUpdatesBackfill {
		return h.showUpdatesPrompt(c, user)
	}
	if err = h.HandleToday(c); err != nil {
		return err
	}
	return c.Send("Настройка завершена.", h.mainMenu(c))
}

func (h *Handler) showDailySetup(c tele.Context, step string, onboarding bool) error {
	ctx, cancel := reqCtx()
	defer cancel()
	state, user, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil || state == nil {
		return c.Send("Сначала настройте расписание через /start.")
	}
	state.DailyOnboarding = onboarding
	state.FlowNonce = newFlowNonce()
	state.Step = "daily_choice"
	menu := &tele.ReplyMarkup{}
	text := "Хотите получать расписание на сегодня каждый день?"
	if step == "time" {
		state.Step = "daily_time"
		menu.Inline(menu.Row(menu.Data("06:00", "daily_action", "06:00", state.FlowNonce), menu.Data("07:00", "daily_action", "07:00", state.FlowNonce), menu.Data("08:00", "daily_action", "08:00", state.FlowNonce)),
			menu.Row(menu.Data("Другое время", "daily_action", "custom", state.FlowNonce)), menu.Row(menu.Data("Назад", "daily_action", "back", state.FlowNonce)))
		text = "Выберите время или отправьте его в формате ЧЧ:ММ, например 06:00.\nЧасовой пояс: " + h.universityLocation(ctx, state.UniversityID).String() + ".\nЕжедневная отправка выполняется и во время тихих часов."
	} else if onboarding {
		menu.Inline(menu.Row(menu.Data("Да, настроить время", "daily_action", "on", state.FlowNonce)), menu.Row(menu.Data("Нет, позже", "daily_action", "off", state.FlowNonce)))
	} else {
		text = "Ежедневное расписание: " + onOff(user.DailyEnabled) + ".\nВремя: " + user.DailyTime + " · " + h.universityLocation(ctx, state.UniversityID).String() + ".\nИзменить время: /daily 06:00. Отключить: /daily off.\nОтправка выполняется и во время тихих часов."
		menu.Inline(menu.Row(menu.Data("Настроить время", "daily_action", "on", state.FlowNonce)), menu.Row(menu.Data("Выключить", "daily_action", "off", state.FlowNonce)), menu.Row(menu.Data("Закрыть", "daily_action", "close", state.FlowNonce)))
	}
	h.StateManager.Set(c.Sender().ID, state)
	return c.Send(text, menu)
}

func (h *Handler) HandleDaily(c tele.Context) error {
	if c.Callback() != nil {
		_ = c.Respond()
	}
	value := strings.TrimSpace(strings.Join(c.Args(), " "))
	ctx, cancel := reqCtx()
	defer cancel()
	state, user, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil || state == nil {
		return c.Send("Сначала настройте расписание через /start.")
	}
	onboarding := user.DailySetup == "choice" || user.DailySetup == "time"
	if value == "" {
		return h.showDailySetup(c, "choice", onboarding)
	}
	state.DailyOnboarding = onboarding
	return h.saveDaily(c, state, value)
}

func (h *Handler) HandleDailyAction(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) != 2 || state == nil || (state.Step != "daily_choice" && state.Step != "daily_time") || !validFlow(state, state.Step, args[1]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	switch args[0] {
	case "on", "back":
		step := "time"
		if args[0] == "back" {
			step = "choice"
		}
		if state.DailyOnboarding {
			if err := h.ProfileService.SetDailySetup(ctx, fmt.Sprint(c.Sender().ID), step); err != nil {
				return c.Send("Не удалось сохранить настройку. Попробуйте ещё раз.")
			}
		}
		return h.showDailySetup(c, step, state.DailyOnboarding)
	case "custom":
		if state.Step != "daily_time" {
			return respondStaleCallback(c)
		}
		return c.Send("Введите время в формате ЧЧ:ММ, например 06:30.")
	case "close":
		if state.DailyOnboarding {
			return respondStaleCallback(c)
		}
		return h.HandleMenu(c)
	default:
		if args[0] != "off" && state.Step != "daily_time" {
			return respondStaleCallback(c)
		}
		return h.saveDaily(c, state, args[0])
	}
}

func (h *Handler) saveDaily(c tele.Context, state *dto.UserState, value string) error {
	ctx, cancel := reqCtx()
	defer cancel()
	enabled := value != "off"
	clock := value
	if !enabled {
		user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
		if err != nil || user == nil {
			return c.Send("Не удалось загрузить настройки.")
		}
		clock = user.DailyTime
		if clock == "" {
			clock = "06:00"
		}
	}
	if err := h.ProfileService.SetDailySchedule(ctx, fmt.Sprint(c.Sender().ID), enabled, clock); err != nil {
		return c.Send("Не удалось сохранить время. Укажите ЧЧ:ММ от 00:00 до 23:59 или /daily off.")
	}
	text := "Ежедневная отправка выключена. Её можно включить в настройках или через /daily."
	if enabled {
		text = "Буду присылать расписание на сегодня каждый день в " + clock + " по времени вуза (" + h.universityLocation(ctx, state.UniversityID).String() + ")."
	}
	if err := c.Send(text); err != nil {
		return err
	}
	if state.DailyOnboarding {
		return h.finishProfileSetup(c)
	}
	if _, _, err := h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return err
	}
	return c.Send("Настройки сохранены.", keyboards.BackButton("open_main_menu"))
}
