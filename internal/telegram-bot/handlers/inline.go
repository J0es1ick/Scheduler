package handlers

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"

	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleInlineQuery(c tele.Context) error {
	query := c.Query()
	if query == nil || query.Sender == nil {
		return nil
	}
	ctx, cancel := reqCtx()
	defer cancel()

	user, err := h.UserService.GetUser(ctx, fmt.Sprint(query.Sender.ID))
	if err != nil {
		return c.Answer(inlineUnavailableResponse())
	}
	if user == nil || user.DefaultGroupID == "" {
		return c.Answer(inlineSetupResponse())
	}
	target, err := h.downloadTarget(ctx, c, keyboards.GroupToken(user.DefaultGroupID))
	if err != nil {
		return c.Answer(inlineUnavailableResponse())
	}

	location := h.universityLocation(ctx, target.UniversityID)
	now := time.Now().In(location)
	dates, ok := inlineQueryDates(query.Text, now)
	if !ok {
		return c.Answer(&tele.QueryResponse{
			Results:           tele.Results{},
			CacheTime:         1,
			IsPersonal:        true,
			SwitchPMText:      "Открыть выбор даты",
			SwitchPMParameter: "date",
		})
	}

	results := make(tele.Results, 0, len(dates))
	freshness := h.sourceFreshnessText(target.UniversityID)
	for _, date := range dates {
		days, loadErr := h.getScheduleForTarget(ctx, target, date, date)
		if loadErr != nil {
			return c.Answer(inlineUnavailableResponse())
		}
		if len(days) == 0 {
			continue
		}
		article := &tele.ArticleResult{
			Title:       inlineDateTitle(date, now),
			Description: fmt.Sprintf("%s · Занятий: %d", target.GroupName, len(days[0].Lessons)),
		}
		article.SetResultID(date.Format("20060102"))
		article.SetContent(&tele.InputTextMessageContent{Text: inlineScheduleText(target, days[0], freshness), ParseMode: tele.ModeHTML})
		results = append(results, article)
	}

	return c.Answer(&tele.QueryResponse{
		Results:           results,
		CacheTime:         30,
		IsPersonal:        true,
		SwitchPMText:      "Открыть бота",
		SwitchPMParameter: "menu",
	})
}

func inlineScheduleText(target *scheduleTarget, day dto.DaySchedule, freshness string) string {
	header := fmt.Sprintf("%s · Группа: %s", html.EscapeString(target.University), html.EscapeString(target.GroupName))
	if target.Subgroup > 0 {
		header += fmt.Sprintf(" · Подгруппа: %d", target.Subgroup)
	}
	best := "Дата: " + day.Date.Format("02.01.2006") + "\nПолное расписание откройте в личном чате с ботом."
	for count := 0; count <= len(day.Lessons); count++ {
		visible := day
		visible.Lessons = day.Lessons[:count]
		body := formatDaySchedule(visible)
		tail := freshness
		if count < len(day.Lessons) {
			if count == 0 {
				body = "Дата: " + day.Date.Format("02.01.2006") + "\n"
			}
			tail += fmt.Sprintf("\nПоказано занятий: %d из %d. Полное расписание откройте в личном чате с ботом.", count, len(day.Lessons))
		}
		text := header + "\n\n" + body + tail
		if utf8.RuneCountInString(text) > tgMaxLen {
			break
		}
		best = text
	}
	return best
}

func inlineUnavailableResponse() *tele.QueryResponse {
	return &tele.QueryResponse{Results: tele.Results{}, CacheTime: 1, IsPersonal: true, SwitchPMText: "Проверить доступность расписания", SwitchPMParameter: "menu"}
}

func inlineSetupResponse() *tele.QueryResponse {
	return &tele.QueryResponse{
		Results:           tele.Results{},
		CacheTime:         1,
		IsPersonal:        true,
		SwitchPMText:      "Сначала выбрать группу",
		SwitchPMParameter: "setup",
	}
}

func inlineQueryDates(input string, now time.Time) ([]time.Time, bool) {
	input = strings.ToLower(strings.TrimSpace(input))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch input {
	case "", "расписание":
		return []time.Time{today, today.AddDate(0, 0, 1)}, true
	case "сегодня", "today":
		return []time.Time{today}, true
	case "завтра", "tomorrow":
		return []time.Time{today.AddDate(0, 0, 1)}, true
	}
	for _, layout := range []string{"02.01.2006", "2006-01-02"} {
		date, err := time.ParseInLocation(layout, input, now.Location())
		if err == nil {
			return []time.Time{date}, true
		}
	}
	return nil, false
}

func inlineDateTitle(date, now time.Time) string {
	today := now.In(date.Location())
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	switch {
	case date.Equal(today):
		return "Расписание на сегодня"
	case date.Equal(today.AddDate(0, 0, 1)):
		return "Расписание на завтра"
	default:
		return "Расписание на " + date.Format("02.01.2006")
	}
}
