//go:build integration

package handlers

import (
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"

	tele "gopkg.in/telebot.v3"
)

func (j *runtimeJourney) pressAction(t *testing.T, action string, handler tele.HandlerFunc, match func(string) bool) {
	t.Helper()
	var args string
	found := false
	j.telegram.mu.Lock()
	for _, row := range j.telegram.markup.InlineKeyboard {
		for _, button := range row {
			name, data, _ := strings.Cut(strings.TrimPrefix(button.Data, "\f"), "|")
			if name == action && (match == nil || match(data)) {
				args = data
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	j.telegram.mu.Unlock()
	if !found {
		t.Fatalf("missing generated action %s", action)
	}
	j.callback(t, handler, args)
}

func TestRuntimeGeneratedScheduleButtonsRestoreContext(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	j.callback(t, j.h.HandleSetScheduleView, "journey-a|compact|0")
	j.callback(t, j.h.HandleSetSubscriptionSubgroup, "journey-a|1|0")
	j.message(t, j.h.HandleToday, "/today", "")
	j.pressAction(t, "open_calendar", j.h.HandleOpenCalendar, nil)
	j.h.StateManager.Delete(42)
	j.pressAction(t, "calendar_month", j.h.HandleCalendarMonth, nil)
	j.pressAction(t, "schedule_date", j.h.HandleScheduleDateSelect, nil)
	j.pressAction(t, "schedule_week", j.h.HandleScheduleWeekSelect, nil)
	j.pressAction(t, "open_weekday", j.h.HandleOpenWeekday, nil)
	j.pressAction(t, "schedule_period_date", j.h.HandleSchedulePeriodDateSelect, nil)
	j.pressAction(t, "schedule_week", j.h.HandleScheduleWeekSelect, nil)
	j.pressAction(t, "schedule_week", j.h.HandleScheduleWeekSelect, func(args string) bool { return strings.Contains(args, "|14|") })
	if strings.Contains(j.telegram.text, "Предмет подгруппы 2") {
		t.Fatal("navigation lost subgroup")
	}
	j.pressAction(t, "open_schedule_exports", j.h.HandleOpenScheduleExports, nil)
	j.pressAction(t, "download_schedule", j.h.HandleDownloadSchedule, func(args string) bool { return strings.HasPrefix(args, "csv|") })
	if len(j.telegram.files) == 0 {
		t.Fatal("navigation export missing")
	}
	j.pressAction(t, "open_schedule_exports", j.h.HandleOpenScheduleExports, nil)
	j.pressAction(t, "back_to_schedule", j.h.HandleBackToSchedule, nil)
	j.pressAction(t, "open_main_menu", j.h.HandleOpenMainMenu, nil)
	j.message(t, j.h.HandleToday, "/today", "")
	if !strings.Contains(j.telegram.text, "Предмет подгруппы 1") || strings.Contains(j.telegram.text, "Предмет подгруппы 2") {
		t.Fatal("profile lost after calendar and export navigation")
	}
}

func TestRuntimeInterruptedSetupAndStaleTeacherButtons(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	j.message(t, j.h.HandleChangeUniversity, "/change_university", "")
	j.pressAction(t, "select_university", j.h.HandleUniversitySelect, func(args string) bool { return strings.HasPrefix(args, keyboards.UniversityToken("isuct")+"|") })
	j.pressAction(t, "back_university_selection", j.h.HandleBackUniversitySelection, nil)
	j.pressAction(t, "cancel_university_selection", j.h.HandleCancelUniversitySelection, nil)
	if _, err := j.db.Exec(`UPDATE lessons SET teacher='Иванов П.П.' WHERE subgroup=2`); err != nil {
		t.Fatal(err)
	}
	j.message(t, j.h.HandleSearch, "/search", "")
	j.pressAction(t, "select_search_type", j.h.HandleSearchTypeSelect, func(args string) bool { return strings.HasPrefix(args, "teacher|") })
	j.message(t, j.h.HandleTextInput, "Иванов", "")
	oldNonce := j.h.StateManager.Get(42).FlowNonce
	j.message(t, j.h.HandleHotline, "/hotline", "")
	currentNonce := j.h.StateManager.Get(42).FlowNonce
	j.callback(t, j.h.HandleTeacherSelect, "0|"+oldNonce)
	if current := j.h.StateManager.Get(42); current.FlowNonce != currentNonce || current.Step != "choosing_hotline_type" {
		t.Fatalf("old teacher button replaced current dialog: %+v", current)
	}
	j.pressAction(t, "cancel_hotline_type", j.h.HandleCancelHotlineType, nil)
	j.message(t, j.h.HandleSearch, "/search", "")
	j.pressAction(t, "select_search_type", j.h.HandleSearchTypeSelect, func(args string) bool { return strings.HasPrefix(args, "teacher|") })
	j.message(t, j.h.HandleTextInput, "Иванов", "")
	j.pressAction(t, "select_teacher", j.h.HandleTeacherSelect, nil)
	j.pressAction(t, "open_schedule_exports", j.h.HandleOpenScheduleExports, nil)
	j.pressAction(t, "download_schedule", j.h.HandleDownloadSchedule, func(args string) bool { return strings.HasPrefix(args, "ics|") })
	j.message(t, j.h.HandleStart, "/start", "")
	user, err := j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user.DefaultGroupID != "journey-a" {
		t.Fatalf("cancelled setup changed profile: %+v %v", user, err)
	}
}
