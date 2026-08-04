package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	if input == "" {
		now := time.Now()
		return c.Send(
			calendarTitle(now),
			keyboards.ScheduleCalendar(now),
		)
	}
	date, err := parseScheduleDate(input, time.Local)
	if err != nil {
		return c.Send(
			"Не удалось распознать дату. Используйте формат ДД.ММ.ГГГГ, " +
				"например /date 01.09.2026.",
		)
	}
	return h.sendTargetDate(ctx, c, target, date)
}

func (h *Handler) HandleCalendarMonth(c tele.Context) error {
	defer c.Respond()
	args := c.Args()
	if len(args) == 0 {
		return nil
	}
	month, err := time.ParseInLocation("2006-01", args[0], time.Local)
	if err != nil || month.Year() < 2000 || month.Year() > 2100 {
		return c.Send("Некорректный месяц.")
	}
	if err = c.Edit(
		calendarTitle(month),
		keyboards.ScheduleCalendar(month),
	); err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return c.Send(calendarTitle(month), keyboards.ScheduleCalendar(month))
	}
	return nil
}

func (h *Handler) HandleScheduleDateSelect(c tele.Context) error {
	defer c.Respond()
	args := c.Args()
	if len(args) == 0 {
		return nil
	}
	date, err := parseScheduleDate(args[0], time.Local)
	if err != nil {
		return c.Send("Некорректная дата.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
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
	days := h.getScheduleForTarget(ctx, target, date, date)
	if len(days) == 0 || len(days[0].Lessons) == 0 {
		return c.Send(fmt.Sprintf(
			"%s, %s\nЗанятий нет.",
			weekdayNames[weekdayNumber(date)],
			date.Format("02.01.2006"),
		))
	}
	if err := h.sendSingleDay(c, days[0], target.UniversityID); err != nil {
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
