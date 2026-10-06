package handlers

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/miniapp"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleStart(c tele.Context) error {
	if isGroupChat(c) {
		ctx, cancel := reqCtx()
		defer cancel()
		return c.Send(
			"Я могу показывать расписание прямо в этом чате.\n\n" +
				h.chatGroupSetupHint(ctx) + "\n\n" +
				"После настройки участникам будут доступны /today, /tomorrow, " +
				"/week, /twoweeks и /date.",
		)
	}

	if args := c.Args(); len(args) == 1 && strings.HasPrefix(args[0], "report_") {
		return h.handleReportLink(c, args[0])
	}
	ctx, cancel := reqCtx()
	defer cancel()

	telegramID := fmt.Sprint(c.Sender().ID)
	user, err := h.UserService.RegisterOrGetUser(ctx, telegramID, c.Sender().Username)
	if err != nil {
		slog.Error("user register failed", "telegramID", telegramID, "err", err)
		return c.Send("Не удалось открыть профиль. Попробуйте ещё раз позже.")
	}
	if err = h.configureMiniAppMenu(ctx, c.Bot(), c.Sender(), user.IsAdmin); err != nil {
		slog.Debug("menu button configuration skipped", "user_id", user.ID, "err", err)
	} else if err = h.UserService.MarkTelegramMenuConfigured(
		ctx, user.ID, miniapp.MenuFingerprint(h.AdminPublicURL, user.IsAdmin),
	); err != nil {
		slog.Warn("save menu button state failed", "user_id", user.ID, "err", err)
	}

	name := c.Sender().FirstName
	if name == "" {
		name = "друг"
	}
	greeting := fmt.Sprintf("%s, %s!", timeGreeting(), name)

	state, profile, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore user profile failed", "user_id", user.ID, "err", err)
		return c.Send("Не удалось загрузить сохранённое расписание. Попробуйте ещё раз позже.")
	}
	if state != nil {
		if profile.DailySetup == "choice" || profile.DailySetup == "time" {
			return h.showDailySetup(c, profile.DailySetup, true)
		}
		if h.UpdatesService != nil && profile.ServiceUpdatesConsent == nil && !profile.ServiceUpdatesBackfill {
			return h.showUpdatesPrompt(c, profile)
		}
		payload := ""
		if len(c.Args()) > 0 {
			payload = strings.ToLower(strings.TrimSpace(c.Args()[0]))
		}
		switch payload {
		case "date":
			now := time.Now().In(h.universityLocation(ctx, state.UniversityID))
			return c.Send(calendarTitle(now), keyboards.ScheduleCalendar(now))
		case "menu":
			return h.HandleMenu(c)
		case "setup":
			return h.HandleChangeUniversity(c)
		}
		if state.Role == domain.RoleTeacher {
			availability := ""
			if !state.GroupActive {
				availability = "\nРасписание временно недоступно; привязка сохранена."
			}
			return c.Send(greeting+"\n\nМоё расписание: "+state.University+" · "+state.TeacherName+availability, h.mainMenu(c))
		}
		availability := ""
		if !state.GroupActive {
			availability = "\nСтатус: группа временно не публикуется. Выбор сохранён; можно переключиться в «Мои группы»."
		}
		return c.Send(fmt.Sprintf(
			"%s\n\nОсновная группа: %s · %s%s\nУправление подписками: кнопка «Мои группы»",
			greeting,
			state.University,
			state.Query,
			availability,
		), h.mainMenu(c))
	}

	return h.HandleRole(c)
}

func timeGreeting() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 6 && hour < 12:
		return "Доброе утро"
	case hour >= 12 && hour < 18:
		return "Добрый день"
	case hour >= 18 && hour < 23:
		return "Добрый вечер"
	default:
		return "Доброй ночи"
	}
}
