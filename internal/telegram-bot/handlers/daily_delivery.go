package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scheduleview"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	tele "gopkg.in/telebot.v3"
)

var errScheduleUnavailable = errors.New("schedule unavailable")

func (h *Handler) personalScheduleTarget(ctx context.Context, userID string) (*scheduleTarget, error) {
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errScheduleUnavailable
	}
	target := &scheduleTarget{ViewFormat: domain.ScheduleViewVisual}
	if user.Role == domain.RoleTeacher {
		teacher, loadErr := h.ProfileService.GetTeacher(ctx, user.TeacherID)
		if loadErr != nil {
			return nil, loadErr
		}
		if teacher == nil {
			return nil, errScheduleUnavailable
		}
		target.UniversityID = teacher.UniversityID
		target.TeacherName = teacher.Name
		target.GroupName = teacher.Name
		target.ViewFormat = user.TeacherScheduleView
		names, loadErr := h.ScheduleService.FindTeachers(ctx, teacher.UniversityID, "")
		if loadErr != nil {
			return nil, loadErr
		}
		found := false
		for _, name := range names {
			if searchtext.TokenKey(name) == teacher.NameKey {
				found = true
				break
			}
		}
		if !found {
			return nil, errScheduleUnavailable
		}
	} else {
		group, loadErr := h.GroupService.GetGroupByID(ctx, user.DefaultGroupID)
		if loadErr != nil {
			return nil, loadErr
		}
		if group == nil || !group.IsActive {
			return nil, errScheduleUnavailable
		}
		target.GroupID = group.ID
		target.GroupName = group.Name
		target.UniversityID = group.UniversityID
		subscriptions, loadErr := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
		if loadErr != nil {
			return nil, loadErr
		}
		for _, subscription := range subscriptions {
			if subscription.GroupID == group.ID {
				target.ViewFormat = subscription.ScheduleViewFormat
				target.Subgroup = subscription.Subgroup
				break
			}
		}
	}
	university, err := h.UniversityService.GetByID(ctx, target.UniversityID)
	if err != nil {
		return nil, err
	}
	if university == nil || !university.IsActive {
		return nil, errScheduleUnavailable
	}
	target.University = university.Name
	return target, nil
}

func (h *Handler) PrepareDailySchedule(ctx context.Context, userID string, date time.Time) ([]domain.ScheduleMessage, error) {
	target, err := h.personalScheduleTarget(ctx, userID)
	if err != nil {
		if !errors.Is(err, errScheduleUnavailable) {
			return nil, err
		}
		return []domain.ScheduleMessage{{Text: "Расписание на " + date.Format("02.01.2006") + " сейчас недоступно. Проверьте настройки или повторите /today позже."}}, nil
	}
	days, err := h.getScheduleForTarget(ctx, target, date, date)
	if err != nil {
		return nil, err
	}
	return h.prepareScheduleMessages(ctx, target, days, date, 1, scheduleDayNavigationForTarget(date, target, false), "")
}

func (h *Handler) prepareScheduleMessages(ctx context.Context, target *scheduleTarget, days []dto.DaySchedule, from time.Time, count int, markup *tele.ReplyMarkup, header string) ([]domain.ScheduleMessage, error) {
	rawMarkup, err := json.Marshal(markup)
	if err != nil {
		return nil, err
	}
	header = target.decorateScheduleHeader(header)
	foot := h.sourceFreshnessText(target.UniversityID) + "\nСообщить об ошибке: /report"
	hasLessons := false
	for _, day := range days {
		if len(day.Lessons) > 0 {
			hasLessons = true
			break
		}
	}
	emptyText := ""
	if !hasLessons {
		freshness, loadErr := h.UniversityService.GetSourceFreshness(ctx, target.UniversityID)
		if loadErr != nil {
			return nil, loadErr
		}
		message := "Занятий нет."
		if from.In(h.universityLocation(ctx, target.UniversityID)).Format(time.DateOnly) == time.Now().In(h.universityLocation(ctx, target.UniversityID)).Format(time.DateOnly) {
			message = "Сегодня занятий нет."
		}
		if count != 1 {
			message = formatScheduleDays(days, target.showGroupNames(), from, count)
		}
		if freshness != nil && freshness.LastSuccess == nil {
			message = "Расписание пока не опубликовано. Попробуйте после обновления источника."
		}
		emptyText = strings.TrimSpace(header + "\n" + formatSchedulePeriodHTML(from, count) + "\n" + message + foot)
		if freshness != nil && freshness.LastSuccess == nil {
			return []domain.ScheduleMessage{{Text: emptyText, Markup: rawMarkup}}, nil
		}
	}
	if target.ViewFormat == domain.ScheduleViewVisual {
		png, renderErr := scheduleview.RenderPNGContext(ctx, h.visualScheduleRequest(ctx, target, days, from, count))
		if renderErr == nil {
			caption := target.decorateScheduleHeader(formatSchedulePeriodHTML(from, count)) + foot
			if emptyText != "" {
				caption = emptyText
			}
			return []domain.ScheduleMessage{{PNG: png, Text: caption, Markup: rawMarkup}}, nil
		}
	}
	if emptyText != "" {
		return []domain.ScheduleMessage{{Text: emptyText, Markup: rawMarkup}}, nil
	}
	text := strings.TrimSpace(header + "\n\n" + formatScheduleDays(days, target.showGroupNames(), from, count) + foot)
	parts := service.SplitMessage(text, tgMaxLen)
	result := make([]domain.ScheduleMessage, len(parts))
	for i, part := range parts {
		result[i].Text = part
		if i == len(parts)-1 {
			result[i].Markup = rawMarkup
		}
	}
	return result, nil
}
