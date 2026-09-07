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
		wd = 7
	}
	header := fmt.Sprintf("<i>%s, %s</i>\n", weekdayNames[wd], day.Date.Format("02.01.2006"))

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
			"%s–%s · <b>%s</b>\n<i>%s%s</i>\n",
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

func (h *Handler) sendDays(c tgbotapi.Context, days []dto.DaySchedule, universityID string) error {
	return h.sendDaysWithMarkup(c, days, universityID, nil)
}

func (h *Handler) sendDaysWithGroupNames(c tgbotapi.Context, days []dto.DaySchedule, universityID string) error {
	return h.sendDaysWithOptions(c, days, universityID, nil, "", true, time.Time{}, 0)
}

func (h *Handler) sendDaysWithMarkup(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
) error {
	return h.sendDaysWithOptions(c, days, universityID, markup, "", false, time.Time{}, 0)
}

func (h *Handler) sendDaysWithOptions(
	c tgbotapi.Context,
	days []dto.DaySchedule,
	universityID string,
	markup *tgbotapi.ReplyMarkup,
	header string,
	showGroupNames bool,
	from time.Time,
	daysCount int,
) error {
	footer := h.sourceFreshnessText(universityID) + scheduleReportLink(c)
	header = strings.TrimSpace(header)
	if header != "" {
		header += "\n\n"
	}
	hasLessons := false
	for _, day := range days {
		if len(day.Lessons) > 0 {
			hasLessons = true
			break
		}
	}
	if !hasLessons {
		emptySchedule := formatScheduleDays(days, showGroupNames, from, daysCount)
		if len(days) == 0 && daysCount != 14 {
			emptySchedule = "Занятий нет."
		}
		freshnessCtx, cancel := reqCtx()
		freshness, freshnessErr := h.UniversityService.GetSourceFreshness(freshnessCtx, universityID)
		cancel()
		if freshnessErr != nil {
			emptySchedule = "Не удалось проверить наличие публикации. Повторите запрос позже."
		} else if freshness != nil && freshness.LastSuccess == nil {
			emptySchedule = "Расписание пока не опубликовано. Попробуйте после обновления источника."
		}
		text := header + emptySchedule + footer
		if c.Callback() != nil && markup != nil {
			if h.hasTrackedScheduleMessages(c) {
				return h.replaceTrackedScheduleMessages(c, []string{text}, markup)
			}
			return editOrSendHTML(c, text, markup)
		}
		if markup != nil {
			return h.sendScheduleMessage(c, text, markup)
		}
		return c.Send(text, markup, tgbotapi.ModeHTML)
	}

	var full strings.Builder
	full.WriteString(header)
	full.WriteString(formatScheduleDays(days, showGroupNames, from, daysCount))
	full.WriteString(footer)

	parts := service.SplitMessage(full.String(), tgMaxLen)
	if len(parts) == 1 && c.Callback() != nil && markup != nil {
		if h.hasTrackedScheduleMessages(c) {
			return h.replaceTrackedScheduleMessages(c, parts, markup)
		}
		return editOrSendHTML(c, parts[0], markup)
	}
	if len(parts) > 1 && markup != nil {
		return h.replaceTrackedScheduleMessages(c, parts, markup)
	}
	for index, part := range parts {
		var err error
		if index == len(parts)-1 && markup != nil {
			err = h.sendScheduleMessage(c, part, markup)
		} else {
			err = c.Send(part, tgbotapi.ModeHTML)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func formatScheduleDays(
	days []dto.DaySchedule,
	showGroupNames bool,
	from time.Time,
	daysCount int,
) string {
	if daysCount == 14 {
		boundary := from.AddDate(0, 0, 7)
		return formatScheduleWeekSection("Первая неделя", days, showGroupNames, func(day dto.DaySchedule) bool {
			return day.Date.Before(boundary)
		}) + formatScheduleWeekSection("Вторая неделя", days, showGroupNames, func(day dto.DaySchedule) bool {
			return !day.Date.Before(boundary)
		})
	}
	var full strings.Builder
	for _, day := range days {
		if showGroupNames {
			full.WriteString(formatDayScheduleWithGroupNames(day))
		} else {
			full.WriteString(formatDaySchedule(day))
		}
		full.WriteString("\n")
	}
	return full.String()
}

func formatScheduleWeekSection(
	title string,
	days []dto.DaySchedule,
	showGroupNames bool,
	include func(dto.DaySchedule) bool,
) string {
	var section strings.Builder
	section.WriteString("<b>" + title + "</b>\n\n")
	written := false
	for _, day := range days {
		if !include(day) {
			continue
		}
		written = true
		if showGroupNames {
			section.WriteString(formatDayScheduleWithGroupNames(day))
		} else {
			section.WriteString(formatDaySchedule(day))
		}
		section.WriteString("\n")
	}
	if !written {
		section.WriteString("Занятий нет.\n\n")
	}
	return section.String()
}

func (h *Handler) hasTrackedScheduleMessages(c tgbotapi.Context) bool {
	key := scheduleMessagesKey(c)
	if key == "" {
		return false
	}
	h.scheduleMessagesMu.Lock()
	defer h.scheduleMessagesMu.Unlock()
	_, ok := h.scheduleMessages[key]
	return ok
}

func (h *Handler) replaceTrackedScheduleMessages(
	c tgbotapi.Context,
	parts []string,
	markup *tgbotapi.ReplyMarkup,
) error {
	ctx, cancel := scheduleDeliveryContext(len(parts) + 1)
	defer cancel()
	var notice *tgbotapi.Message
	var err error
	if !isGroupChat(c) && c.Callback() == nil {
		notice, err = h.sendTelegram(
			ctx,
			c,
			c.Recipient(),
			"Открываю расписание…",
			&tgbotapi.ReplyMarkup{RemoveKeyboard: true},
		)
		if err != nil {
			return err
		}
		defer func() { _, _ = h.deleteScheduleMessages(c, []*tgbotapi.Message{notice}) }()
	}
	sent := make([]*tgbotapi.Message, 0, len(parts))
	for index, part := range parts {
		options := []interface{}{tgbotapi.ModeHTML}
		if index == len(parts)-1 {
			options = append(options, markup)
		}
		message, sendErr := h.sendTelegram(ctx, c, c.Recipient(), part, options...)
		if sendErr != nil {
			remaining, cleanupErr := h.deleteScheduleMessages(c, sent)
			h.rememberTrackedScheduleMessages(c, remaining)
			if len(remaining) > 0 {
				recoveryCtx, recoveryCancel := reqCtx()
				cleanupErr = errors.Join(cleanupErr, h.editTelegramMarkup(recoveryCtx, c, remaining[len(remaining)-1], markup))
				recoveryCancel()
			}
			return errors.Join(sendErr, cleanupErr)
		}
		sent = append(sent, message)
	}
	if c.Callback() != nil {
		if cleanupErr := h.deleteTrackedScheduleMessages(c); cleanupErr != nil {
			slog.Warn("previous schedule cleanup incomplete", "err", cleanupErr)
			h.scheduleMessagesMu.Lock()
			remaining := h.scheduleMessages[scheduleMessagesKey(c)]
			delete(h.scheduleMessages, scheduleMessagesKey(c))
			h.scheduleMessagesMu.Unlock()
			retained := make([]*tgbotapi.Message, 0, len(remaining.IDs)+len(sent))
			for _, id := range remaining.IDs {
				retained = append(retained, &tgbotapi.Message{ID: id, Chat: c.Chat()})
			}
			sent = append(retained, sent...)
		}
	}
	h.rememberTrackedScheduleMessages(c, sent)
	return nil
}

func scheduleDeliveryContext(operations int) (context.Context, context.CancelFunc) {
	budget := min(2*time.Minute, time.Duration(max(1, operations))*handlerTimeout)
	return context.WithTimeout(context.Background(), budget)
}

func (h *Handler) deleteScheduleMessages(c tgbotapi.Context, messages []*tgbotapi.Message) ([]*tgbotapi.Message, error) {
	ctx, cancel := scheduleDeliveryContext(len(messages))
	defer cancel()
	remaining := make([]*tgbotapi.Message, 0)
	var deleteErrors []error
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		operationCtx, operationCancel := context.WithTimeout(ctx, handlerTimeout)
		err := h.deleteTelegram(operationCtx, c, message)
		operationCancel()
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "message to delete not found") {
			remaining = append(remaining, message)
			deleteErrors = append(deleteErrors, fmt.Errorf("delete schedule message %d: %w", message.ID, err))
		}
	}
	return remaining, errors.Join(deleteErrors...)
}

func (h *Handler) deleteTrackedScheduleMessages(c tgbotapi.Context) error {
	key := scheduleMessagesKey(c)
	h.scheduleMessagesMu.Lock()
	tracked, ok := h.scheduleMessages[key]
	h.scheduleMessagesMu.Unlock()
	if !ok {
		return retireInlineMessage(c, "Обновляю расписание…")
	}
	messages := make([]*tgbotapi.Message, 0, len(tracked.IDs))
	for _, id := range tracked.IDs {
		messages = append(messages, &tgbotapi.Message{ID: id, Chat: c.Chat()})
	}
	remaining, err := h.deleteScheduleMessages(c, messages)
	h.scheduleMessagesMu.Lock()
	for _, id := range tracked.IDs {
		delete(h.scheduleMessages, fmt.Sprintf("%d:%d", c.Chat().ID, id))
	}
	if len(remaining) > 0 {
		ids := make([]int, 0, len(remaining))
		for _, message := range remaining {
			ids = append(ids, message.ID)
		}
		h.scheduleMessages[key] = trackedScheduleMessages{IDs: ids, CreatedAt: tracked.CreatedAt}
	}
	h.scheduleMessagesMu.Unlock()
	return err
}

func (h *Handler) rememberTrackedScheduleMessages(c tgbotapi.Context, messages []*tgbotapi.Message) {
	if len(messages) == 0 || c.Chat() == nil {
		return
	}
	ids := make([]int, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	cutoff := time.Now().Add(-48 * time.Hour)
	h.scheduleMessagesMu.Lock()
	if h.scheduleMessages == nil {
		h.scheduleMessages = make(map[string]trackedScheduleMessages)
	}
	for storedKey, tracked := range h.scheduleMessages {
		if tracked.CreatedAt.Before(cutoff) {
			delete(h.scheduleMessages, storedKey)
		}
	}
	for _, id := range ids {
		key := fmt.Sprintf("%d:%d", c.Chat().ID, id)
		h.scheduleMessages[key] = trackedScheduleMessages{IDs: ids, CreatedAt: time.Now()}
	}
	h.scheduleMessagesMu.Unlock()
}

func scheduleMessagesKey(c tgbotapi.Context) string {
	if c.Callback() == nil || c.Message() == nil || c.Chat() == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", c.Chat().ID, c.Message().ID)
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
	label := "Опубликовано: "
	if freshness.State == "stale" {
		result += "\nОбновление задерживается. Данные могут быть устаревшими."
	}
	if freshness.State == "error" {
		result += "\nИсточник временно недоступен; показана последняя публикация."
	}
	if freshness.State == "disabled" {
		result += "\nАвтообновление приостановлено."
	}
	return result + "\n" + label + freshness.LastSuccess.In(h.universityLocation(ctx, universityID)).Format("02.01.2006 15:04 MST")
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
	ctx context.Context,
	c tgbotapi.Context,
	day dto.DaySchedule,
	target *scheduleTarget,
) error {
	markup := scheduleDayNavigationForTarget(day.Date, target, isGroupChat(c))
	return h.sendScheduleView(ctx, c, []dto.DaySchedule{day}, target, day.Date, 1, markup, "")
}

func scheduleDayNavigationForTarget(date time.Time, target *scheduleTarget, groupChat bool) *tgbotapi.ReplyMarkup {
	if target.TeacherName != "" {
		return keyboards.TeacherScheduleDayNavigation(date, target.TeacherName, target.navigationReference())
	}
	return keyboards.ScheduleDayNavigation(date, target.GroupName, groupChat, target.GroupID, target.navigationReference())
}

func scheduleWeekNavigationForTarget(
	from time.Time,
	target *scheduleTarget,
	groupChat bool,
	daysCount int,
) *tgbotapi.ReplyMarkup {
	if target.TeacherName != "" {
		return keyboards.TeacherScheduleWeekNavigation(from, target.TeacherName, daysCount, target.navigationReference())
	}
	return keyboards.ScheduleWeekNavigation(from, target.GroupName, groupChat, target.GroupID, daysCount, target.navigationReference())
}

func (h *Handler) sendScheduleMessage(
	c tgbotapi.Context,
	text string,
	markup *tgbotapi.ReplyMarkup,
) error {
	if isGroupChat(c) {
		return c.Send(text, markup, tgbotapi.ModeHTML)
	}

	ctx, cancel := reqCtx()
	defer cancel()
	keyboardNotice, err := h.sendTelegram(
		ctx,
		c,
		c.Recipient(),
		"Открываю расписание…",
		&tgbotapi.ReplyMarkup{RemoveKeyboard: true},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := h.deleteTelegram(ctx, c, keyboardNotice); err != nil {
			slog.Debug("delete keyboard notice failed", "err", err)
		}
	}()

	if _, err = h.sendTelegram(ctx, c, c.Recipient(), text, markup, tgbotapi.ModeHTML); err != nil {
		return err
	}
	return nil
}

func (h *Handler) getScheduleForTarget(
	ctx context.Context,
	target *scheduleTarget,
	from time.Time,
	to time.Time,
) ([]dto.DaySchedule, error) {
	if target.TeacherName != "" {
		data, err := h.ScheduleService.GetScheduleForTeacherRange(
			ctx,
			target.UniversityID,
			target.TeacherName,
			from,
			to,
		)
		if err != nil {
			slog.Error("GetScheduleForTeacherRange failed", "teacher", target.TeacherName, "err", err)
			return nil, err
		}
		return mapToDaySchedule(data), nil
	}
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
	if target.Subgroup > 0 {
		for date, dayLessons := range data {
			lessons := dayLessons[:0]
			for _, lesson := range dayLessons {
				if lesson.Subgroup == 0 || lesson.Subgroup == target.Subgroup {
					lessons = append(lessons, lesson)
				}
			}
			data[date] = lessons
		}
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
		return h.sendEmptyTargetDate(ctx, c, target, now)
	}
	return h.sendSingleDayForTarget(ctx, c, days[0], target)
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
		return h.sendEmptyTargetDate(ctx, c, target, tomorrow)
	}
	return h.sendSingleDayForTarget(ctx, c, days[0], target)
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
	if detachedScheduleMenu(c, "Выберите формат файла:") {
		_ = c.Respond()
		return c.Delete()
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleCallbackTarget(ctx, c, args, 2)
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

func detachedScheduleMenu(c tgbotapi.Context, texts ...string) bool {
	message := c.Message()
	if message == nil || messageSupportsCaption(message) {
		return false
	}
	current := strings.TrimSpace(message.Text)
	for _, text := range texts {
		if current == text {
			return true
		}
	}
	return false
}

func (h *Handler) sendTargetWeek(
	ctx context.Context,
	c tgbotapi.Context,
	target *scheduleTarget,
	from time.Time,
	daysCount int,
) error {
	from = scheduleWeekStart(from)
	markup := scheduleWeekNavigationForTarget(from, target, isGroupChat(c), daysCount)
	days, err := h.getScheduleForTarget(ctx, target, from, from.AddDate(0, 0, daysCount-1))
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	return h.sendScheduleView(ctx, c, days, target, from, daysCount, markup, formatSchedulePeriodHTML(from, daysCount))
}

func editOrSendHTML(c tgbotapi.Context, text string, markup *tgbotapi.ReplyMarkup) error {
	return editOrReplace(c, text, markup, tgbotapi.ModeHTML)
}

func (h *Handler) sendScheduleView(
	ctx context.Context,
	c tgbotapi.Context,
	days []dto.DaySchedule,
	target *scheduleTarget,
	from time.Time,
	daysCount int,
	markup *tgbotapi.ReplyMarkup,
	header string,
) error {
	header = target.decorateScheduleHeader(header)
	if target.ViewFormat != domain.ScheduleViewVisual {
		return h.sendDaysWithOptions(c, days, target.UniversityID, markup, header, target.showGroupNames(), from, daysCount)
	}
	payload, err := scheduleview.RenderPNGContext(ctx, h.visualScheduleRequest(ctx, target, days, from, daysCount))
	if err != nil {
		slog.Error("render visual schedule failed", "group_id", target.GroupID, "err", err)
		return h.sendDaysWithOptions(c, days, target.UniversityID, markup, header, target.showGroupNames(), from, daysCount)
	}
	photo := &tgbotapi.Photo{
		File:    tgbotapi.FromReader(bytes.NewReader(payload)),
		Caption: target.decorateScheduleHeader(formatSchedulePeriodHTML(from, daysCount)) + h.sourceFreshnessText(target.UniversityID) + scheduleReportLink(c),
	}
	if c.Callback() != nil {
		if messageSupportsCaption(c.Message()) {
			if err = c.Edit(photo, markup, tgbotapi.ModeHTML); err == nil ||
				strings.Contains(err.Error(), "message is not modified") {
				return nil
			}
			slog.Debug("edit previous schedule media failed", "err", err)
		}
		if err = c.Delete(); err != nil {
			return fmt.Errorf("replace previous schedule message: %w", err)
		}
		return c.Send(photo, markup, tgbotapi.ModeHTML)
	}
	return h.sendScheduleMedia(c, photo, markup)
}

func (h *Handler) sendScheduleMedia(c tgbotapi.Context, media tgbotapi.Sendable, markup *tgbotapi.ReplyMarkup) error {
	if isGroupChat(c) {
		return c.Send(media, markup, tgbotapi.ModeHTML)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	notice, err := h.sendTelegram(ctx, c, c.Recipient(), "Открываю расписание…", &tgbotapi.ReplyMarkup{RemoveKeyboard: true})
	if err != nil {
		return err
	}
	defer func() { _ = h.deleteTelegram(ctx, c, notice) }()
	_, err = h.sendTelegram(ctx, c, c.Recipient(), media, markup, tgbotapi.ModeHTML)
	return err
}

func schedulePNG(ctx context.Context, target *scheduleTarget, days []dto.DaySchedule, from time.Time, daysCount int) ([]byte, error) {
	return scheduleview.RenderPNGContext(ctx, scheduleRenderRequest(target, days, from, daysCount))
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
		University:              target.University,
		Group:                   target.displayName(),
		From:                    from,
		Days:                    daysCount,
		Schedule:                renderDays,
		ShowGroupNames:          target.showGroupNames(),
		NormalizeResearchBlocks: target.UniversityID == "isuct",
	}
}

func (target *scheduleTarget) decorateScheduleHeader(header string) string {
	if target.TeacherName == "" {
		return header
	}
	prefix := "<b>Преподаватель: " + html.EscapeString(target.TeacherName) + "</b>"
	if strings.TrimSpace(header) == "" {
		return prefix
	}
	return prefix + "\n" + header
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
	return h.handleDownloadSchedule(c, "png", callbackArguments(c))
}

func (h *Handler) HandleDownloadScheduleICS(c tgbotapi.Context) error {
	return h.handleDownloadSchedule(c, "ics", callbackArguments(c))
}

func (h *Handler) HandleOpenScheduleExports(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) < 3 {
		return respondStaleCallback(c)
	}
	daysCount, err := strconv.Atoi(args[2])
	if err != nil || daysCount < 1 || daysCount > 14 {
		return respondStaleCallback(c)
	}
	if _, err = parseScheduleDate(args[1], time.Local); err != nil {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	return editScheduleOverlay(
		c,
		"Выберите формат файла:",
		keyboards.ScheduleExportFormats(args[0], args[1], daysCount),
	)
}

func (h *Handler) HandleBackToSchedule(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) < 3 {
		return respondStaleCallback(c)
	}
	daysCount, err := strconv.Atoi(args[2])
	if err != nil || daysCount < 1 || daysCount > 14 {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target, err := h.downloadTarget(ctx, c, args[0])
	if err != nil || target == nil {
		return editOrSend(
			c,
			"Доступ к этой группе изменился. Откройте актуальное расписание заново.",
			keyboards.BackButton("close_inline"),
		)
	}
	from, err := parseScheduleDate(args[1], h.universityLocation(ctx, target.UniversityID))
	if err != nil {
		return respondStaleCallback(c)
	}
	to := from.AddDate(0, 0, daysCount-1)
	days, err := h.getScheduleForTarget(ctx, target, from, to)
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	var markup *tgbotapi.ReplyMarkup
	if daysCount == 1 {
		markup = scheduleDayNavigationForTarget(from, target, isGroupChat(c))
	} else {
		markup = scheduleWeekNavigationForTarget(from, target, isGroupChat(c), daysCount)
	}
	return h.sendScheduleView(ctx, c, days, target, from, daysCount, markup, formatSchedulePeriodHTML(from, daysCount))
}

func (h *Handler) HandleDownloadSchedule(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) < 4 {
		return respondStaleCallback(c)
	}
	return h.handleDownloadSchedule(c, args[0], args[1:])
}

func (h *Handler) handleDownloadSchedule(c tgbotapi.Context, format string, args []string) error {
	if len(args) < 3 || (format != "png" && format != "json" && format != "csv" && format != "ics") {
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
	var (
		payload   []byte
		extension string
	)
	switch format {
	case "png":
		request = h.visualScheduleRequest(ctx, target, days, from, daysCount)
		payload, err = scheduleview.RenderPNGContext(ctx, request)
		extension = ".png"
	case "json":
		payload, err = scheduleview.RenderJSON(request)
		extension = ".json"
	case "csv":
		payload, err = scheduleview.RenderCSV(request)
		extension = ".csv"
	case "ics":
		payload, err = scheduleview.RenderICS(request)
		extension = ".ics"
	}
	if err != nil {
		slog.Error("render schedule download failed", "group_id", target.GroupID, "err", err)
		return c.Send("Не удалось подготовить файл. Попробуйте позже.")
	}
	document := &tgbotapi.Document{
		File:     tgbotapi.FromReader(bytes.NewReader(payload)),
		FileName: scheduleFileName(target.displayName(), from, daysCount, extension),
		Caption: fmt.Sprintf(
			"%s · %s\n%s",
			target.University,
			target.displayName(),
			formatSchedulePeriod(from, daysCount),
		),
	}
	if format == "ics" {
		document.Caption += "\nICS — разовый импорт, не обновляемая подписка. При изменении расписания скачайте файл заново."
	}
	sent, err := h.sendTelegram(
		ctx,
		c,
		c.Recipient(),
		document,
		keyboards.ScheduleExportResultNavigation(args[0], args[1], daysCount),
	)
	if err != nil {
		return err
	}
	if c.Callback() != nil {
		if retireErr := h.retireCurrentInlineFlow(c, "Файл подготовлен."); retireErr != nil {
			_ = h.deleteTelegram(ctx, c, sent)
			return fmt.Errorf("retire export format menu: %w", retireErr)
		}
	}
	return nil
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
	if strings.HasPrefix(groupToken, "p") && len(groupToken) == 17 {
		group, err := h.GroupService.GetActiveGroupByToken(ctx, strings.TrimPrefix(groupToken, "p"))
		if err != nil {
			return nil, fmt.Errorf("load public group: %w", err)
		}
		if group == nil || !group.IsActive {
			return nil, errors.New("public group not found")
		}
		university, err := h.UniversityService.GetByID(ctx, group.UniversityID)
		if err != nil {
			return nil, fmt.Errorf("load public university: %w", err)
		}
		if university == nil || !university.IsActive {
			return nil, errors.New("public university not found")
		}
		return &scheduleTarget{
			GroupID: group.ID, GroupName: group.Name, UniversityID: group.UniversityID,
			University: university.Name, ViewFormat: domain.ScheduleViewVisual, Public: true,
		}, nil
	}
	if strings.HasPrefix(groupToken, "t") && len(groupToken) == 17 {
		user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
		if err != nil || user == nil || user.DefaultGroupID == "" {
			return nil, errors.New("teacher search profile not found")
		}
		group, err := h.GroupService.GetGroupByID(ctx, user.DefaultGroupID)
		if err != nil || group == nil || !group.IsActive {
			return nil, errors.New("teacher search university not found")
		}
		names, err := h.ScheduleService.FindTeachers(ctx, group.UniversityID, "")
		if err != nil {
			return nil, fmt.Errorf("load teachers: %w", err)
		}
		for _, name := range names {
			if keyboards.TeacherToken(group.UniversityID, name) == groupToken {
				return h.teacherScheduleTarget(ctx, c, group.UniversityID, name)
			}
		}
		return nil, errors.New("teacher not found")
	}
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if keyboards.GroupToken(item.GroupID) == groupToken && item.IsActive {
			return &scheduleTarget{
				GroupID:      item.GroupID,
				GroupName:    item.GroupName,
				UniversityID: item.UniversityID,
				University:   item.UniversityName,
				ViewFormat:   item.ScheduleViewFormat,
				Subgroup:     item.Subgroup,
			}, nil
		}
	}
	return nil, errors.New("subscription not found")
}

func (h *Handler) scheduleCallbackTarget(ctx context.Context, c tgbotapi.Context, args []string, index int) *scheduleTarget {
	if len(args) <= index || args[index] == "" {
		return h.scheduleTarget(ctx, c)
	}
	target, err := h.downloadTarget(ctx, c, args[index])
	if err != nil {
		slog.Warn("resolve schedule navigation target failed", "user_id", c.Sender().ID, "err", err)
		_ = editOrSend(c, "Это расписание больше недоступно. Откройте его заново.", keyboards.BackButton("open_main_menu"))
		return nil
	}
	return target
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
	if daysCount == 1 {
		return "Дата: " + from.Format("02.01.2006")
	}
	to := from.AddDate(0, 0, daysCount-1)
	label := "Период"
	if daysCount == 7 {
		label = "Неделя"
	} else if daysCount == 14 {
		label = "Две недели"
	}
	return fmt.Sprintf("%s: %s–%s", label, from.Format("02.01.2006"), to.Format("02.01.2006"))
}

func formatSchedulePeriodHTML(from time.Time, daysCount int) string {
	return "<b>" + html.EscapeString(formatSchedulePeriod(from, daysCount)) + "</b>"
}

func (h *Handler) HandleWeekDay(c tgbotapi.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleTarget(ctx, c)
	if target == nil {
		return nil
	}
	return c.Send("Выберите день недели:", keyboards.WeekDaySelector(h.targetNow(ctx, target), 7, target.navigationReference()))
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

func (h *Handler) HandleSchedulePeriodDateSelect(c tgbotapi.Context) error {
	args := callbackArguments(c)
	if len(args) < 3 {
		return respondStaleCallback(c)
	}
	daysCount, err := strconv.Atoi(args[2])
	if err != nil || (daysCount != 7 && daysCount != 14) {
		return respondStaleCallback(c)
	}
	if _, err = parseScheduleDate(args[0], time.Local); err != nil {
		return respondStaleCallback(c)
	}
	if _, err = parseScheduleDate(args[1], time.Local); err != nil {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleCallbackTarget(ctx, c, args, 3)
	if target == nil {
		return nil
	}
	location := h.universityLocation(ctx, target.UniversityID)
	date, err := parseScheduleDate(args[0], location)
	if err != nil {
		return respondStaleCallback(c)
	}
	from, err := parseScheduleDate(args[1], location)
	if err != nil {
		return nil
	}
	days, err := h.getScheduleForTarget(ctx, target, date, date)
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	markup := scheduleDayNavigationForTarget(date, target, isGroupChat(c))
	back := keyboards.SchedulePeriodDateBack(from, daysCount, target.navigationReference())
	markup.InlineKeyboard = append(markup.InlineKeyboard, back.InlineKeyboard...)
	return h.sendScheduleView(
		ctx, c, days, target, date, 1,
		markup,
		formatSchedulePeriodHTML(date, 1),
	)
}
