package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

// tgMaxLen — максимальная длина одного Telegram-сообщения в символах.
const tgMaxLen = 4096

var weekdayNames = []string{"", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

func formatDaySchedule(day dto.DaySchedule) string {
	wd := int(day.Date.Weekday())
	if wd == 0 {
		wd = 7 // воскресенье
	}
	header := fmt.Sprintf("%s, %s\n", weekdayNames[wd], day.Date.Format("02.01.2006"))

	if len(day.Lessons) == 0 {
		return header + "Занятий нет.\n"
	}

	var sb strings.Builder
	sb.WriteString(header)
	for _, l := range day.Lessons {
		subgroup := ""
		if l.Subgroup > 0 {
			subgroup = fmt.Sprintf(", подгруппа %d", l.Subgroup)
		}
		sb.WriteString(fmt.Sprintf(
			"  %s–%s | %s (%s%s)\n  %s | %s\n\n",
			l.TimeStart, l.TimeEnd,
			l.Subject, lessonTypeLabel(l.Type), subgroup,
			l.Teacher, l.Room,
		))
	}
	return sb.String()
}

func lessonTypeLabel(lessonType domain.LessonType) string {
	switch lessonType {
	case domain.LessonTypeLecture:
		return "лекция"
	case domain.LessonTypePractice:
		return "практика"
	case domain.LessonTypeLab:
		return "лабораторная"
	case domain.LessonTypeExam:
		return "экзамен"
	case domain.LessonTypeCredit:
		return "зачёт"
	case domain.LessonTypeConsultation:
		return "консультация"
	default:
		return "семинар"
	}
}

// sendDays форматирует расписание и отправляет его, разбивая на части
// если текст превышает лимит Telegram.
func (h *Handler) sendDays(c tgbotapi.Context, days []dto.DaySchedule, universityID string) error {
	return h.sendDaysWithMarkup(c, days, universityID, nil)
}

func (h *Handler) sendDaysWithMarkup(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
) error {
	if len(days) == 0 {
		if c.Callback() != nil && markup != nil {
			return editOrSend(c, "Занятий нет.", markup)
		}
		if markup != nil {
			return sendScheduleMessage(c, "Занятий нет.", markup)
		}
		return c.Send("Занятий нет.", markup)
	}

	var full strings.Builder
	for _, day := range days {
		full.WriteString(formatDaySchedule(day))
		full.WriteString("\n")
	}
	full.WriteString(h.sourceFreshnessText(universityID))

	parts := service.SplitMessage(full.String(), tgMaxLen)
	if len(parts) == 1 && c.Callback() != nil && markup != nil {
		return editOrSend(c, parts[0], markup)
	}
	for index, part := range parts {
		var err error
		if index == len(parts)-1 && markup != nil {
			err = sendScheduleMessage(c, part, markup)
		} else {
			err = c.Send(part)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) sourceFreshnessText(universityID string) string {
	ctx, cancel := reqCtx()
	defer cancel()
	freshness, err := h.UniversityService.GetSourceFreshness(ctx, universityID)
	if err != nil || freshness == nil {
		return "\nИсточник: данные сервиса; время обновления недоступно."
	}
	result := "\nИсточник: " + freshness.ScheduleURL
	if freshness.LastSuccess == nil {
		return result + "\nПоследнее успешное обновление ещё не зафиксировано."
	}
	return result + "\nОбновлено: " + freshness.LastSuccess.In(h.universityLocation(ctx, universityID)).Format("02.01.2006 15:04 MST")
}

func (h *Handler) universityLocation(ctx context.Context, universityID string) *time.Location {
	university, err := h.UniversityService.GetByID(ctx, universityID)
	if err != nil || university == nil || university.Timezone == "" {
		return time.Local
	}
	location, err := time.LoadLocation(university.Timezone)
	if err != nil {
		return time.Local
	}
	return location
}

func (h *Handler) targetNow(ctx context.Context, target *scheduleTarget) time.Time {
	return time.Now().In(h.universityLocation(ctx, target.UniversityID))
}

func (h *Handler) sendSingleDayForTarget(
	c tgbotapi.Context,
	day dto.DaySchedule,
	target *scheduleTarget,
) error {
	text := formatDaySchedule(day) + h.sourceFreshnessText(target.UniversityID)
	markup := keyboards.ScheduleDayNavigation(day.Date, target.GroupName, isGroupChat(c))
	if c.Callback() != nil {
		return editOrSend(c, text, markup)
	}
	return sendScheduleMessage(c, text, markup)
}

func sendScheduleMessage(
	c tgbotapi.Context,
	text string,
	markup *tgbotapi.ReplyMarkup,
) error {
	if isGroupChat(c) {
		return c.Send(text, markup)
	}

	keyboardNotice, err := c.Bot().Send(
		c.Recipient(),
		"Открываю расписание…",
		&tgbotapi.ReplyMarkup{RemoveKeyboard: true},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Bot().Delete(keyboardNotice); err != nil {
			slog.Debug("delete keyboard notice failed", "err", err)
		}
	}()

	if _, err = c.Bot().Send(c.Recipient(), text, markup); err != nil {
		return err
	}
	return nil
}

// getScheduleForTarget загружает расписание группы за диапазон дат.
// Принимает уже созданный ctx — вызывающий хэндлер владеет его таймаутом/отменой.
func (h *Handler) getScheduleForTarget(
	ctx context.Context,
	target *scheduleTarget,
	from time.Time,
	to time.Time,
) ([]dto.DaySchedule, error) {
	data, err := h.ScheduleService.GetScheduleForGroupRange(
		ctx,
		target.GroupID,
		from,
		to,
	)
	if err != nil {
		slog.Error(
			"GetScheduleForGroupRange failed",
			"groupID", target.GroupID,
			"err", err,
		)
		return nil, err
	}
	return mapToDaySchedule(data), nil
}

func sendScheduleLoadError(c tgbotapi.Context, err error) error {
	slog.Error("schedule is temporarily unavailable", "err", err)
	const message = "Не удалось загрузить расписание. Попробуйте ещё раз через несколько минут."
	if c.Callback() != nil {
		if respondErr := c.Respond(&tgbotapi.CallbackResponse{Text: message, ShowAlert: true}); respondErr != nil {
			return errors.Join(err, respondErr)
		}
		return nil
	}
	if sendErr := c.Send(message); sendErr != nil {
		return errors.Join(err, sendErr)
	}
	return nil
}

func (h *Handler) HandleToday(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	now := h.targetNow(ctx, target)
	days, err := h.getScheduleForTarget(ctx, target, now, now)
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	if len(days) == 0 {
		return h.sendEmptyTargetDate(c, target, now)
	}
	return h.sendSingleDayForTarget(c, days[0], target)
}

func (h *Handler) HandleTomorrow(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	tomorrow := h.targetNow(ctx, target).AddDate(0, 0, 1)
	days, err := h.getScheduleForTarget(ctx, target, tomorrow, tomorrow)
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	if len(days) == 0 {
		return h.sendEmptyTargetDate(c, target, tomorrow)
	}
	return h.sendSingleDayForTarget(c, days[0], target)
}

func (h *Handler) HandleWeek(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	return h.sendTargetWeek(ctx, c, target, h.targetNow(ctx, target), 7)
}

func (h *Handler) HandleTwoWeeks(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	return h.sendTargetWeek(ctx, c, target, h.targetNow(ctx, target), 14)
}

func (h *Handler) HandleScheduleWeekSelect(c tgbotapi.Context) error {
	value, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}
	from, err := parseScheduleDate(value, h.universityLocation(ctx, target.UniversityID))
	if err != nil {
		return respondStaleCallback(c)
	}
	return h.sendTargetWeek(ctx, c, target, from, 7)
}

func (h *Handler) sendTargetWeek(
	ctx context.Context,
	c tgbotapi.Context,
	target *scheduleTarget,
	from time.Time,
	daysCount int,
) error {
	markup := keyboards.ScheduleWeekNavigation(from, target.GroupName, isGroupChat(c))
	days, err := h.getScheduleForTarget(ctx, target, from, from.AddDate(0, 0, daysCount-1))
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	return h.sendDaysWithMarkup(
		c,
		days,
		target.UniversityID,
		markup,
	)
}

func (h *Handler) HandleWeekDay(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}
	return c.Send("Выберите день недели:", keyboards.WeekDaySelector(h.targetNow(ctx, target)))
}

func (h *Handler) HandleWeekDaySelect(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) == 0 {
		return respondStaleCallback(c)
	}

	var weekdayNum int
	fmt.Sscanf(args[0], "%d", &weekdayNum)
	if weekdayNum < 1 || weekdayNum > 7 {
		return respondStaleCallback(c)
	}
	_ = c.Respond()

	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	location := h.universityLocation(ctx, target.UniversityID)
	from := time.Now().In(location)
	if len(args) > 1 {
		parsed, err := parseScheduleDate(args[1], location)
		if err == nil {
			from = parsed
		}
	}
	selectedDate := dateAtLocation(from, location)
	for offset := 0; offset < 7; offset++ {
		candidate := selectedDate.AddDate(0, 0, offset)
		if weekdayNumber(candidate) == weekdayNum {
			return h.sendTargetDate(ctx, c, target, candidate)
		}
	}
	return respondStaleCallback(c)
}
