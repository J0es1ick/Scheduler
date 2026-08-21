package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scheduleview"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tgbotapi "gopkg.in/telebot.v3"
)

// tgMaxLen — максимальная длина одного Telegram-сообщения в символах.
const tgMaxLen = 4096

var weekdayNames = []string{"", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

func formatDaySchedule(day dto.DaySchedule) string {
	return formatDayScheduleWithOptions(day, false)
}

func formatDayScheduleWithGroupNames(day dto.DaySchedule) string {
	return formatDayScheduleWithOptions(day, true)
}

func formatDayScheduleWithOptions(day dto.DaySchedule, showGroupNames bool) string {
	wd := int(day.Date.Weekday())
	if wd == 0 {
		wd = 7 // воскресенье
	}
	header := fmt.Sprintf("<b>%s, %s</b>\n", weekdayNames[wd], day.Date.Format("02.01.2006"))

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
		details := make([]string, 0, 3)
		if showGroupNames && strings.TrimSpace(l.GroupName) != "" {
			details = append(details, "группа "+html.EscapeString(l.GroupName))
		}
		if strings.TrimSpace(l.Teacher) != "" {
			details = append(details, html.EscapeString(l.Teacher))
		}
		if strings.TrimSpace(l.Room) != "" {
			details = append(details, html.EscapeString(l.Room))
		}
		sb.WriteString(fmt.Sprintf(
			"%s <b>%s–%s</b> · %s\n<i>%s%s</i>\n",
			lessonTypeMarker(l.Type),
			l.TimeStart, l.TimeEnd,
			html.EscapeString(l.Subject), lessonTypeLabel(l.Type), html.EscapeString(subgroup),
		))
		if len(details) > 0 {
			sb.WriteString("  " + strings.Join(details, " · ") + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func lessonTypeMarker(lessonType domain.LessonType) string {
	switch lessonType {
	case domain.LessonTypeLecture:
		return "🟩"
	case domain.LessonTypePractice:
		return "🩷"
	case domain.LessonTypeLab:
		return "🟦"
	case domain.LessonTypeSeminar:
		return "🟨"
	case domain.LessonTypeExam, domain.LessonTypeCredit:
		return "🟥"
	case domain.LessonTypeConsultation:
		return "🟪"
	default:
		return "⬜"
	}
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
	case domain.LessonTypeOther:
		return "занятие"
	default:
		return "семинар"
	}
}

// sendDays форматирует расписание и отправляет его, разбивая на части
// если текст превышает лимит Telegram.
func (h *Handler) sendDays(c tgbotapi.Context, days []dto.DaySchedule, universityID string) error {
	return h.sendDaysWithMarkup(c, days, universityID, nil)
}

func (h *Handler) sendDaysWithGroupNames(c tgbotapi.Context, days []dto.DaySchedule, universityID string) error {
	return h.sendDaysWithOptions(c, days, universityID, nil, "", true)
}

func (h *Handler) sendDaysWithMarkup(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
) error {
	return h.sendDaysWithOptions(c, days, universityID, markup, "", false)
}

func (h *Handler) sendDaysWithMarkupAndHeader(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
	header string,
) error {
	return h.sendDaysWithOptions(c, days, universityID, markup, header, false)
}

func (h *Handler) sendDaysWithOptions(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
	header string,
	showGroupNames bool,
) error {
	header = strings.TrimSpace(header)
	if header != "" {
		header += "\n\n"
	}
	if len(days) == 0 {
		text := header + "Занятий нет." + h.sourceFreshnessText(universityID)
		if c.Callback() != nil && markup != nil {
			return editOrSendHTML(c, text, markup)
		}
		if markup != nil {
			return sendScheduleMessage(c, text, markup)
		}
		return c.Send(text, markup, tgbotapi.ModeHTML)
	}

	var full strings.Builder
	full.WriteString(header)
	for _, day := range days {
		if showGroupNames {
			full.WriteString(formatDayScheduleWithGroupNames(day))
		} else {
			full.WriteString(formatDaySchedule(day))
		}
		full.WriteString("\n")
	}
	full.WriteString(h.sourceFreshnessText(universityID))

	parts := service.SplitMessage(full.String(), tgMaxLen)
	if len(parts) == 1 && c.Callback() != nil && markup != nil {
		return editOrSendHTML(c, parts[0], markup)
	}
	for index, part := range parts {
		var err error
		if index == len(parts)-1 && markup != nil {
			err = sendScheduleMessage(c, part, markup)
		} else {
			err = c.Send(part, tgbotapi.ModeHTML)
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
	result := "\nИсточник: " + html.EscapeString(freshness.ScheduleURL)
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
	markup := keyboards.ScheduleDayNavigation(day.Date, target.GroupName, isGroupChat(c), target.GroupID)
	return h.sendScheduleView(c, []dto.DaySchedule{day}, target, day.Date, 1, markup, "")
}

func sendScheduleMessage(
	c tgbotapi.Context,
	text string,
	markup *tgbotapi.ReplyMarkup,
) error {
	if isGroupChat(c) {
		return c.Send(text, markup, tgbotapi.ModeHTML)
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

	if _, err = c.Bot().Send(c.Recipient(), text, markup, tgbotapi.ModeHTML); err != nil {
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
	args := callbackArguments(c)
	if len(args) == 0 {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}
	from, err := parseScheduleDate(args[0], h.universityLocation(ctx, target.UniversityID))
	if err != nil {
		return respondStaleCallback(c)
	}
	daysCount := 7
	if len(args) > 1 && args[1] == "14" {
		daysCount = 14
	}
	return h.sendTargetWeek(ctx, c, target, from, daysCount)
}

func (h *Handler) sendTargetWeek(
	ctx context.Context,
	c tgbotapi.Context,
	target *scheduleTarget,
	from time.Time,
	daysCount int,
) error {
	from = scheduleWeekStart(from)
	markup := keyboards.ScheduleWeekNavigation(from, target.GroupName, isGroupChat(c), target.GroupID, daysCount)
	days, err := h.getScheduleForTarget(ctx, target, from, from.AddDate(0, 0, daysCount-1))
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	return h.sendScheduleView(c, days, target, from, daysCount, markup, formatSchedulePeriod(from, daysCount))
}

func editOrSendHTML(c tgbotapi.Context, text string, markup *tgbotapi.ReplyMarkup) error {
	if err := c.Edit(text, markup, tgbotapi.ModeHTML); err == nil ||
		strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return c.Send(text, markup, tgbotapi.ModeHTML)
}

func (h *Handler) sendScheduleView(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	target *scheduleTarget,
	from time.Time,
	daysCount int,
	markup *tgbotapi.ReplyMarkup,
	header string,
) error {
	if target.ViewFormat != domain.ScheduleViewVisual || isGroupChat(c) {
		return h.sendDaysWithMarkupAndHeader(c, days, target.UniversityID, markup, header)
	}
	payload, err := schedulePNG(target, days, from, daysCount)
	if err != nil {
		slog.Error("render visual schedule failed", "group_id", target.GroupID, "err", err)
		return h.sendDaysWithMarkupAndHeader(c, days, target.UniversityID, markup, header)
	}
	photo := &tgbotapi.Photo{
		File:    tgbotapi.FromReader(bytes.NewReader(payload)),
		Caption: formatSchedulePeriod(from, daysCount) + h.sourceFreshnessText(target.UniversityID),
	}
	if c.Callback() != nil {
		if err = c.Delete(); err != nil {
			slog.Debug("delete previous schedule message failed", "err", err)
		}
		return c.Send(photo, markup, tgbotapi.ModeHTML)
	}
	return sendScheduleMedia(c, photo, markup)
}

func sendScheduleMedia(c tgbotapi.Context, media tgbotapi.Sendable, markup *tgbotapi.ReplyMarkup) error {
	if isGroupChat(c) {
		return c.Send(media, markup, tgbotapi.ModeHTML)
	}
	notice, err := c.Bot().Send(c.Recipient(), "Открываю расписание…", &tgbotapi.ReplyMarkup{RemoveKeyboard: true})
	if err != nil {
		return err
	}
	defer func() { _ = c.Bot().Delete(notice) }()
	_, err = c.Bot().Send(c.Recipient(), media, markup, tgbotapi.ModeHTML)
	return err
}

func schedulePNG(target *scheduleTarget, days []dto.DaySchedule, from time.Time, daysCount int) ([]byte, error) {
	return scheduleview.RenderPNG(scheduleRenderRequest(target, days, from, daysCount))
}

func scheduleRenderRequest(
	target *scheduleTarget,
	days []dto.DaySchedule,
	from time.Time,
	daysCount int,
) scheduleview.Request {
	renderDays := make([]scheduleview.Day, len(days))
	for index, day := range days {
		renderDays[index] = scheduleview.Day{Date: day.Date, Lessons: day.Lessons}
	}
	return scheduleview.Request{
		University: target.University,
		Group:      target.GroupName,
		From:       from,
		Days:       daysCount,
		Schedule:   renderDays,
	}
}

func scheduleFileName(groupName string, from time.Time, daysCount int, extension string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return fmt.Sprintf(
		"schedule-%s-%s-%dd%s",
		replacer.Replace(groupName),
		from.Format("2006-01-02"),
		daysCount,
		extension,
	)
}

func (h *Handler) HandleDownloadSchedulePNG(c tgbotapi.Context) error {
	return h.handleDownloadSchedule(c, true)
}

func (h *Handler) HandleDownloadScheduleICS(c tgbotapi.Context) error {
	return h.handleDownloadSchedule(c, false)
}

func (h *Handler) handleDownloadSchedule(c tgbotapi.Context, pngFormat bool) error {
	args := callbackArguments(c)
	if len(args) < 3 {
		return respondStaleCallback(c)
	}
	daysCount, err := strconv.Atoi(args[2])
	if err != nil || daysCount < 1 || daysCount > 14 {
		return respondStaleCallback(c)
	}
	_ = c.Respond(&tgbotapi.CallbackResponse{Text: "Готовлю файл…"})
	ctx, cancel := reqCtx()
	defer cancel()
	target, err := h.downloadTarget(ctx, c, args[0])
	if err != nil || target == nil {
		return c.Send("Не удалось подтвердить доступ к этой группе. Откройте актуальное расписание заново.")
	}
	from, err := parseScheduleDate(args[1], h.universityLocation(ctx, target.UniversityID))
	if err != nil {
		return respondStaleCallback(c)
	}
	days, err := h.getScheduleForTarget(ctx, target, from, from.AddDate(0, 0, daysCount-1))
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	request := scheduleRenderRequest(target, days, from, daysCount)
	var payload []byte
	extension := ".ics"
	if pngFormat {
		payload, err = scheduleview.RenderPNG(request)
		extension = ".png"
	} else {
		university, loadErr := h.UniversityService.GetByID(ctx, target.UniversityID)
		if loadErr != nil || university == nil {
			return c.Send("Не удалось определить часовой пояс расписания.")
		}
		payload = scheduleview.RenderICS(request, university.Timezone)
	}
	if err != nil {
		slog.Error("render schedule download failed", "group_id", target.GroupID, "err", err)
		return c.Send("Не удалось подготовить файл. Попробуйте позже.")
	}
	document := &tgbotapi.Document{
		File:     tgbotapi.FromReader(bytes.NewReader(payload)),
		FileName: scheduleFileName(target.GroupName, from, daysCount, extension),
		Caption: fmt.Sprintf(
			"%s · %s\n%s",
			target.University,
			target.GroupName,
			formatSchedulePeriod(from, daysCount),
		),
	}
	return c.Send(document)
}

func (h *Handler) downloadTarget(
	ctx context.Context,
	c tgbotapi.Context,
	groupToken string,
) (*scheduleTarget, error) {
	if isGroupChat(c) {
		target := h.scheduleTarget(ctx, c)
		if target == nil || keyboards.GroupToken(target.GroupID) != groupToken {
			return nil, errors.New("chat group does not match export")
		}
		return target, nil
	}
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if keyboards.GroupToken(item.GroupID) == groupToken {
			return &scheduleTarget{
				GroupID:      item.GroupID,
				GroupName:    item.GroupName,
				UniversityID: item.UniversityID,
				University:   item.UniversityName,
				ViewFormat:   item.ScheduleViewFormat,
			}, nil
		}
	}
	return nil, errors.New("subscription not found")
}

func scheduleWeekStart(date time.Time) time.Time {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	return date.AddDate(0, 0, -(weekday - 1))
}

func formatSchedulePeriod(from time.Time, daysCount int) string {
	to := from.AddDate(0, 0, daysCount-1)
	label := "Период"
	if daysCount == 7 {
		label = "Неделя"
	}
	return fmt.Sprintf("%s: %s–%s", label, from.Format("02.01.2006"), to.Format("02.01.2006"))
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
