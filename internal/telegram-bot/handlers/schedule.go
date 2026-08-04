package handlers

import (
	"context"
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
	if len(days) == 0 {
		return c.Send("Занятий нет.")
	}

	var full strings.Builder
	for _, day := range days {
		full.WriteString(formatDaySchedule(day))
		full.WriteString("\n")
	}
	full.WriteString(h.sourceFreshnessText(universityID))

	for _, part := range service.SplitMessage(full.String(), tgMaxLen) {
		if err := c.Send(part); err != nil {
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
	return result + "\nОбновлено: " + freshness.LastSuccess.In(time.Local).Format("02.01.2006 15:04 MST")
}

func (h *Handler) sendSingleDay(c tgbotapi.Context, day dto.DaySchedule, universityID string) error {
	return c.Send(formatDaySchedule(day) + h.sourceFreshnessText(universityID))
}

// getScheduleForTarget загружает расписание группы за диапазон дат.
// Принимает уже созданный ctx — вызывающий хэндлер владеет его таймаутом/отменой.
func (h *Handler) getScheduleForTarget(
	ctx context.Context,
	target *scheduleTarget,
	from time.Time,
	to time.Time,
) []dto.DaySchedule {
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
		return nil
	}
	return mapToDaySchedule(data)
}

func (h *Handler) HandleToday(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	now := time.Now()
	days := h.getScheduleForTarget(ctx, target, now, now)
	if len(days) == 0 {
		return c.Send("На сегодня занятий нет.")
	}
	return h.sendSingleDay(c, days[0], target.UniversityID)
}

func (h *Handler) HandleTomorrow(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	tomorrow := time.Now().AddDate(0, 0, 1)
	days := h.getScheduleForTarget(ctx, target, tomorrow, tomorrow)
	if len(days) == 0 {
		return c.Send("На завтра занятий нет.")
	}
	return h.sendSingleDay(c, days[0], target.UniversityID)
}

func (h *Handler) HandleWeek(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	now := time.Now()
	return h.sendDays(
		c,
		h.getScheduleForTarget(ctx, target, now, now.AddDate(0, 0, 6)),
		target.UniversityID,
	)
}

func (h *Handler) HandleTwoWeeks(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	now := time.Now()
	return h.sendDays(
		c,
		h.getScheduleForTarget(ctx, target, now, now.AddDate(0, 0, 13)),
		target.UniversityID,
	)
}

func (h *Handler) HandleWeekDay(c tgbotapi.Context) error {
	return c.Send("Выберите день недели:", keyboards.WeekDaySelector())
}

func (h *Handler) HandleWeekDaySelect(c tgbotapi.Context) error {
	args := c.Args()
	if len(args) == 0 {
		_ = c.Respond()
		return c.Send("Некорректный запрос.")
	}

	var weekdayNum int
	fmt.Sscanf(args[0], "%d", &weekdayNum)
	if weekdayNum < 1 || weekdayNum > 7 {
		_ = c.Respond()
		return c.Send("Неверный день недели.")
	}
	_ = c.Respond()

	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}

	now := time.Now()
	days := h.getScheduleForTarget(ctx, target, now, now.AddDate(0, 0, 6))
	for _, day := range days {
		wd := int(day.Date.Weekday())
		if wd == 0 {
			wd = 7
		}
		if wd == weekdayNum {
			return h.sendSingleDay(c, day, target.UniversityID)
		}
	}
	return c.Send("В выбранный день занятий нет.")
}
