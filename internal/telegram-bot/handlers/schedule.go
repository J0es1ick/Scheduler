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
	header := fmt.Sprintf("*%s, %s*\n", weekdayNames[wd], day.Date.Format("02.01.2006"))

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
func (h *Handler) sendDays(c tgbotapi.Context, days []dto.DaySchedule) error {
	if len(days) == 0 {
		return c.Send("Занятий нет.")
	}

	var full strings.Builder
	for _, day := range days {
		full.WriteString(formatDaySchedule(day))
		full.WriteString("\n")
	}

	opts := &tgbotapi.SendOptions{ParseMode: tgbotapi.ModeMarkdown}
	for _, part := range service.SplitMessage(full.String(), tgMaxLen) {
		if err := c.Send(part, opts); err != nil {
			return err
		}
	}
	return nil
}

// getScheduleForState загружает расписание группы за диапазон дат.
// Принимает уже созданный ctx — вызывающий хэндлер владеет его таймаутом/отменой.
func (h *Handler) getScheduleForState(ctx context.Context, state *dto.UserState, from, to time.Time) []dto.DaySchedule {
	if state.SearchType != dto.SearchTypeGroup {
		return nil
	}
	data, err := h.ScheduleService.GetScheduleForGroupRange(ctx, state.GroupID, from, to)
	if err != nil {
		slog.Error("GetScheduleForGroupRange failed", "groupID", state.GroupID, "err", err)
		return nil
	}
	return mapToDaySchedule(data)
}

// checkState проверяет наличие и готовность состояния пользователя.
// Если состояние отсутствует или не завершено — отправляет подсказку и возвращает nil.
func (h *Handler) checkState(c tgbotapi.Context) *dto.UserState {
	ctx, cancel := reqCtx()
	defer cancel()
	state, err := h.readyState(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile failed", "user_id", c.Sender().ID, "err", err)
		_ = c.Send("Не удалось загрузить сохранённую группу. Попробуйте ещё раз позже.")
		return nil
	}
	if state == nil {
		_ = c.Send("Для начала работы используйте /start")
		return nil
	}
	if state.Step != "done" {
		state.Step = "awaiting_query"
		state.SearchType = dto.SearchTypeGroup
		state.Query = ""
		h.StateManager.Set(c.Sender().ID, state)
		remove := &tgbotapi.ReplyMarkup{RemoveKeyboard: true}
		_ = c.Send("Настройка не завершена.", remove)
		_ = c.Send(groupInputPrompt(state.UniversityID))
		return nil
	}
	return state
}

func (h *Handler) HandleToday(c tgbotapi.Context) error {
	state := h.checkState(c)
	if state == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()

	now := time.Now()
	days := h.getScheduleForState(ctx, state, now, now)
	if len(days) == 0 {
		return c.Send("На сегодня занятий нет.")
	}
	return c.Send(
		formatDaySchedule(days[0]),
		&tgbotapi.SendOptions{ParseMode: tgbotapi.ModeMarkdown},
	)
}

func (h *Handler) HandleTomorrow(c tgbotapi.Context) error {
	state := h.checkState(c)
	if state == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()

	tomorrow := time.Now().AddDate(0, 0, 1)
	days := h.getScheduleForState(ctx, state, tomorrow, tomorrow)
	if len(days) == 0 {
		return c.Send("На завтра занятий нет.")
	}
	return c.Send(
		formatDaySchedule(days[0]),
		&tgbotapi.SendOptions{ParseMode: tgbotapi.ModeMarkdown},
	)
}

func (h *Handler) HandleWeek(c tgbotapi.Context) error {
	state := h.checkState(c)
	if state == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()

	now := time.Now()
	return h.sendDays(c, h.getScheduleForState(ctx, state, now, now.AddDate(0, 0, 6)))
}

func (h *Handler) HandleTwoWeeks(c tgbotapi.Context) error {
	state := h.checkState(c)
	if state == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()

	now := time.Now()
	return h.sendDays(c, h.getScheduleForState(ctx, state, now, now.AddDate(0, 0, 13)))
}

func (h *Handler) HandleWeekDay(c tgbotapi.Context) error {
	return c.Send("Выберите день недели:", keyboards.WeekDaySelector())
}

func (h *Handler) HandleWeekDaySelect(c tgbotapi.Context) error {
	state := h.checkState(c)
	if state == nil {
		return nil
	}

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

	now := time.Now()
	days := h.getScheduleForState(ctx, state, now, now.AddDate(0, 0, 6))
	for _, day := range days {
		wd := int(day.Date.Weekday())
		if wd == 0 {
			wd = 7
		}
		if wd == weekdayNum {
			return c.Send(
				formatDaySchedule(day),
				&tgbotapi.SendOptions{ParseMode: tgbotapi.ModeMarkdown},
			)
		}
	}
	return c.Send("В выбранный день занятий нет.")
}
