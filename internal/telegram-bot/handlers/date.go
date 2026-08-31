package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

var calendarMonthNames = []string{
	"",
	"Январь",
	"Февраль",
	"Март",
	"Апрель",
	"Май",
	"Июнь",
	"Июль",
	"Август",
	"Сентябрь",
	"Октябрь",
	"Ноябрь",
	"Декабрь",
}

func (h *Handler) HandleDate(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	input := strings.TrimSpace(strings.Join(c.Args(), " "))
	location := h.universityLocation(ctx, target.UniversityID)
	if input == "" {
		now := time.Now().In(location)
		return c.Send(
			calendarTitle(now),
			keyboards.ScheduleCalendar(now, target.navigationReference()),
		)
	}
	date, err := parseScheduleDate(input, location)
	if err != nil {
		return c.Send(
			"Не удалось распознать дату. Используйте формат ДД.ММ.ГГГГ, " +
				"например /date 01.09.2026.",
		)
	}
	return h.sendTargetDate(ctx, c, target, date)
}

func (h *Handler) HandleCalendarMonth(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) == 0 {
		return respondStaleCallback(c)
	}
	location := time.Local
	if len(args) > 4 && args[4] != "" {
		ctx, cancel := reqCtx()
		defer cancel()
		target := h.scheduleCallbackTarget(ctx, c, args, 4)
		if target == nil {
			return nil
		}
		location = h.universityLocation(ctx, target.UniversityID)
	}
	month, err := time.ParseInLocation("2006-01", args[0], location)
	if err != nil || month.Year() < 2000 || month.Year() > 2100 {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	markup := scheduleCalendarMarkup(month, args)
	return editScheduleOverlay(c, calendarTitle(month), markup)
}

func (h *Handler) HandleOpenCalendar(c tele.Context) error {
	month := time.Now()
	args := callbackArguments(c)
	if len(args) > 4 && args[4] != "" {
		ctx, cancel := reqCtx()
		defer cancel()
		target := h.scheduleCallbackTarget(ctx, c, args, 4)
		if target == nil {
			return nil
		}
		month = h.targetNow(ctx, target)
	}
	if len(args) > 0 {
		parsed, err := time.ParseInLocation("2006-01", args[0], month.Location())
		if err == nil {
			month = parsed
		}
	}
	_ = c.Respond()
	return editScheduleOverlay(c, calendarTitle(month), scheduleCalendarMarkup(month, args))
}

func scheduleCalendarMarkup(month time.Time, args []string) *tele.ReplyMarkup {
	groupToken := ""
	if len(args) > 4 {
		groupToken = args[4]
	}
	if len(args) < 3 {
		return keyboards.ScheduleCalendar(month, groupToken)
	}
	action := args[1]
	if action == "d" {
		action = "schedule_date"
	} else if action == "w" {
		action = "schedule_week"
	}
	if action != "schedule_date" && action != "schedule_week" {
		return keyboards.ScheduleCalendar(month, groupToken)
	}
	backDate, err := parseScheduleDate(args[2], month.Location())
	if err != nil {
		return keyboards.ScheduleCalendar(month, groupToken)
	}
	if len(args) > 3 {
		return keyboards.ScheduleCalendarWithBack(month, action, backDate, args[3], groupToken)
	}
	return keyboards.ScheduleCalendarWithBack(month, action, backDate)
}

func (h *Handler) HandleScheduleDateSelect(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) == 0 {
		return respondStaleCallback(c)
	}
	if detachedScheduleMenu(c, "Выберите формат файла:") {
		_ = c.Respond()
		return c.Delete()
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleCallbackTarget(ctx, c, args, 1)
	if target == nil {
		return nil
	}
	date, err := parseScheduleDate(args[0], h.universityLocation(ctx, target.UniversityID))
	if err != nil {
		return respondStaleCallback(c)
	}
	return h.sendTargetDate(ctx, c, target, date)
}

func (h *Handler) HandleCalendarNoop(c tele.Context) error {
	return c.Respond()
}

func (h *Handler) sendTargetDate(
	ctx context.Context,
	c tele.Context,
	target *scheduleTarget,
	date time.Time,
) error {
	days, err := h.getScheduleForTarget(ctx, target, date, date)
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	if len(days) == 0 || len(days[0].Lessons) == 0 {
		return h.sendEmptyTargetDate(ctx, c, target, date)
	}
	if err := h.sendSingleDayForTarget(ctx, c, days[0], target); err != nil {
		slog.Error(
			"send schedule for selected date failed",
			"group_id", target.GroupID,
			"date", date.Format("2006-01-02"),
			"err", err,
		)
		return err
	}
	return nil
}

func (h *Handler) sendEmptyTargetDate(
	ctx context.Context,
	c tele.Context,
	target *scheduleTarget,
	date time.Time,
) error {
	markup := scheduleDayNavigationForTarget(date, target, isGroupChat(c))
	return h.sendScheduleView(ctx, c, []dto.DaySchedule{{Date: date}}, target, date, 1, markup, "")
}

func parseScheduleDate(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	now := time.Now().In(location)
	switch value {
	case "сегодня":
		return dateAtLocation(now, location), nil
	case "завтра":
		return dateAtLocation(now.AddDate(0, 0, 1), location), nil
	}
	for _, layout := range []string{"02.01.2006", "2006-01-02", "02.01.06"} {
		date, err := time.ParseInLocation(layout, value, location)
		if err == nil {
			if date.Year() < 2000 || date.Year() > 2100 {
				return time.Time{}, fmt.Errorf("year is out of range")
			}
			return dateAtLocation(date, location), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date %q", value)
}

func dateAtLocation(value time.Time, location *time.Location) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

func weekdayNumber(date time.Time) int {
	value := int(date.Weekday())
	if value == 0 {
		return 7
	}
	return value
}

func calendarTitle(month time.Time) string {
	return fmt.Sprintf(
		"Выберите дату · %s %d",
		calendarMonthNames[int(month.Month())],
		month.Year(),
	)
}
