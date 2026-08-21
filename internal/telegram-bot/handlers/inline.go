package handlers

import (
	"fmt"
	"html"
	"strings"
	"time"

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
	if err != nil || user == nil || user.DefaultGroupID == "" {
		return c.Answer(inlineSetupResponse())
	}
	group, err := h.GroupService.GetGroupByID(ctx, user.DefaultGroupID)
	if err != nil || group == nil || !group.IsActive {
		return c.Answer(inlineSetupResponse())
	}

	location := h.universityLocation(ctx, group.UniversityID)
	now := time.Now().In(location)
	dates, ok := inlineQueryDates(query.Text, now)
	if !ok {
		return c.Answer(&tele.QueryResponse{
			Results:           tele.Results{},
			CacheTime:         0,
			IsPersonal:        true,
			SwitchPMText:      "Открыть выбор даты",
			SwitchPMParameter: "date",
		})
	}

	results := make(tele.Results, 0, len(dates))
	for _, date := range dates {
		data, loadErr := h.ScheduleService.GetScheduleForGroupRange(ctx, group.ID, date, date)
		if loadErr != nil {
			continue
		}
		days := mapToDaySchedule(data)
		if len(days) == 0 {
			continue
		}
		text := fmt.Sprintf("Группа: %s\n\n%s%s", html.EscapeString(group.Name), formatDaySchedule(days[0]), h.sourceFreshnessText(group.UniversityID))
		article := &tele.ArticleResult{
			Title:       inlineDateTitle(date, now),
			Description: fmt.Sprintf("%s · %d занятий", group.Name, len(days[0].Lessons)),
			Text:        text,
		}
		article.SetResultID(date.Format("20060102"))
		article.SetParseMode(tele.ModeHTML)
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

func inlineSetupResponse() *tele.QueryResponse {
	return &tele.QueryResponse{
		Results:           tele.Results{},
		CacheTime:         0,
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
