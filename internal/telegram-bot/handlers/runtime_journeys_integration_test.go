//go:build integration

package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	"github.com/jackc/pgx/v5"
	tele "gopkg.in/telebot.v3"
)

func TestRuntimeBotUserJourneys(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	t.Run("commands_and_schedule_formats", func(t *testing.T) {
		for _, format := range []domain.ScheduleViewFormat{domain.ScheduleViewCompact, domain.ScheduleViewVisual} {
			j.callback(t, j.h.HandleSetScheduleView, "journey-a|"+string(format)+"|0")
			for _, command := range []struct {
				name    string
				handler tele.HandlerFunc
			}{
				{"today", j.h.HandleToday}, {"tomorrow", j.h.HandleTomorrow}, {"week", j.h.HandleWeek}, {"twoweeks", j.h.HandleTwoWeeks},
			} {
				t.Run(string(format)+"_"+command.name, func(t *testing.T) { j.message(t, command.handler, "/"+command.name, "") })
			}
		}
		j.message(t, j.h.HandleDate, "/date", j.date.Format("02.01.2006"))
		j.message(t, j.h.HandleWeekDay, "По дню недели", "")
		j.message(t, j.h.HandleMenu, "/menu", "")
		for _, command := range []struct {
			name    string
			handler tele.HandlerFunc
		}{
			{"help", j.h.HandleHelp}, {"privacy", j.h.HandlePrivacy}, {"sources", j.h.HandleSourcesInfo}, {"connect_source", j.h.HandleConnectorInfo}, {"settings", j.h.HandleSettings}, {"reminders", j.h.HandleReminders}, {"quiet_hours", j.h.HandleQuietHours}, {"admin", j.h.HandleAdmin}, {"metrics", j.h.HandleMetrics},
		} {
			t.Run(command.name, func(t *testing.T) { j.message(t, command.handler, "/"+command.name, "") })
		}
	})
	t.Run("settings_and_exports", func(t *testing.T) {
		j.callback(t, j.h.HandleSetSubscriptionSubgroup, "journey-a|1|0")
		j.callback(t, j.h.HandleToggleNotifications, "0")
		j.callback(t, j.h.HandleToggleNotifications, "0")
		j.message(t, j.h.HandleReminders, "/reminders", "45")
		j.callback(t, j.h.HandleSetReminder, "off|0")
		j.message(t, j.h.HandleQuietHours, "/quiet_hours", "22:00-07:00")
		j.message(t, j.h.HandleQuietHours, "/quiet_hours", "off")
		j.callback(t, j.h.HandleSetSearchView, "compact")
		user, err := j.h.UserService.GetUser(j.ctx, "42")
		if err != nil || user.ReminderEnabled || user.QuietHoursEnabled || user.SearchScheduleView != domain.ScheduleViewCompact {
			t.Fatalf("settings not saved: %+v %v", user, err)
		}
		for _, days := range []int{1, 7, 14} {
			for _, format := range []string{"png", "json", "csv", "ics"} {
				t.Run(fmt.Sprintf("%s_%d_days", format, days), func(t *testing.T) {
					args := fmt.Sprintf("%s|%s|%s|%d", format, keyboards.GroupToken("journey-a"), j.date.Format("2006-01-02"), days)
					j.callback(t, j.h.HandleDownloadSchedule, args)
					filename := scheduleFileName("4/147", j.date, days, "."+format)
					payload := j.telegram.files[filename]
					if len(payload) == 0 {
						t.Fatalf("export missing: %s", filename)
					}
					if format != "png" && (strings.Contains(payload, "Предмет подгруппы 2") || !strings.Contains(payload, "Предмет подгруппы 1")) {
						t.Fatalf("subgroup filtering failed in %s", filename)
					}
				})
			}
		}
	})
	t.Run("search_and_navigation", func(t *testing.T) {
		for _, search := range []struct{ kind, query string }{{"group", "ИГХТУ 4/245"}, {"teacher", "Иванов"}, {"room", "А101"}, {"discipline", "Предмет"}} {
			t.Run(search.kind, func(t *testing.T) {
				j.message(t, j.h.HandleSearch, "/search", "")
				j.callback(t, j.h.HandleSearchTypeSelect, search.kind+"|"+j.h.StateManager.Get(42).FlowNonce)
				j.message(t, j.h.HandleTextInput, search.query, "")
				if j.h.StateManager.Get(42).Step != "done" {
					t.Fatalf("search did not finish: %+v", j.h.StateManager.Get(42))
				}
			})
		}
		for _, token := range []string{keyboards.TeacherToken("isuct", "Иванов И.И."), "p" + keyboards.GroupToken("journey-b")} {
			j.callback(t, j.h.HandleScheduleWeekSelect, j.date.Format("2006-01-02")+"|14|"+token)
			j.callback(t, j.h.HandleDownloadSchedule, "ics|"+token+"|"+j.date.Format("2006-01-02")+"|14")
		}
		for _, text := range []string{j.date.Format("02.01.2006"), "пн", "+1", "две недели", "Иванов И.И."} {
			j.message(t, j.h.HandleTextInput, text, "")
		}
	})
	t.Run("subscriptions_and_profile_recovery", func(t *testing.T) {
		j.callback(t, j.h.HandleAddSubscription, "0")
		j.message(t, j.h.HandleTextInput, "ИГХТУ 4/245", "")
		j.callback(t, j.h.HandleSetDefaultSubscription, "journey-b|0")
		j.h.StateManager.Delete(42)
		j.message(t, j.h.HandleStart, "/start", "")
		if j.h.StateManager.Get(42).GroupID != "journey-b" {
			t.Fatal("restart lost primary group")
		}
		j.message(t, j.h.HandleChangeGroup, "/change_group", "")
		j.message(t, j.h.HandleTextInput, "4/147", "")
		j.callback(t, j.h.HandleConfirmPrimaryGroup, "save|"+j.h.StateManager.Get(42).FlowNonce)
		j.callback(t, j.h.HandleRequestDeleteSubscription, "journey-b|0")
		j.callback(t, j.h.HandleConfirmDeleteSubscription, j.telegram.button(t, "Да, удалить"))
		items, err := j.h.SubscriptionService.GetGroupSubscriptions(j.ctx, "42")
		if err != nil || len(items) != 1 || !items[0].IsDefault {
			t.Fatalf("subscription removal failed: %+v %v", items, err)
		}
	})
	t.Run("hotline_privacy_and_background_delivery", func(t *testing.T) {
		for _, kind := range []string{domain.SupportRequestFeedback, domain.SupportRequestNewInstitution, domain.SupportRequestUpdateExisting} {
			j.message(t, j.h.HandleHotline, "/hotline", "")
			j.callback(t, j.h.HandleHotlineType, kind+"|"+j.h.StateManager.Get(42).FlowNonce)
			j.message(t, j.h.HandleTextInput, "Синтетическое обращение для проверки пользовательского сценария: "+kind, "")
			if !strings.Contains(j.telegram.text, "Обращение принято") {
				t.Fatal(j.telegram.text)
			}
		}
		j.message(t, j.h.HandleReport, "/report", "")
		if j.h.StateManager.Get(42).Step != "awaiting_hotline_submission" {
			t.Fatal("report form unavailable")
		}
		j.message(t, j.h.HandleMyData, "/my_data", "")
		data, err := j.h.UserService.ExportData(j.ctx, "42")
		if err != nil || data == nil || len(data.SupportRequests) != 3 || len(data.Subscriptions) != 1 {
			t.Fatalf("export incomplete: %+v %v", data, err)
		}
		queue := repository.NewNotificationRepository(j.botDB)
		outbox, err := queue.ClaimBotOutbox(j.ctx, 20)
		if err != nil || len(outbox) != 3 {
			t.Fatalf("support notifications blocked: %+v %v", outbox, err)
		}
		for _, item := range outbox {
			if err := queue.MarkBotOutboxDelivered(j.ctx, item.ID, item.ClaimToken); err != nil {
				t.Fatal(err)
			}
		}
		if err := repository.NewNotificationRepository(j.db).EnqueueScheduleChange(j.ctx, "journey-event", "journey-a", "parser", "Changed room"); err != nil {
			t.Fatal(err)
		}
		pending, err := queue.ClaimPending(j.ctx, 20)
		if err != nil || len(pending) != 1 {
			t.Fatalf("change notifications blocked: %+v %v", pending, err)
		}
		if err := queue.MarkDelivered(j.ctx, pending[0].ID, pending[0].ClaimToken); err != nil {
			t.Fatal(err)
		}
		j.message(t, j.h.HandleReminders, "/reminders", "30")
		reminders := repository.NewReminderRepository(j.botDB)
		recipients, err := reminders.ActiveRecipientsPage(j.ctx, "", 20)
		if err != nil || len(recipients) != 1 {
			t.Fatalf("reminder recipients unavailable: %+v %v", recipients, err)
		}
		starts := time.Now().Add(15 * time.Minute)
		if err := reminders.Enqueue(j.ctx, "journey-reminder", "42", "journey-a", "Synthetic reminder", domain.ReminderContext{Date: starts.Format("2006-01-02"), TimeStart: starts.Format("15:04"), TimeEnd: starts.Add(time.Hour).Format("15:04"), StartsAt: starts, Subgroup: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.ClaimBotOutbox(j.ctx, 20); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.ReminderRecipient(j.ctx, "42", "journey-a"); err != nil {
			t.Fatal(err)
		}
		if _, err := j.h.MetricsService.Get(j.ctx); err != nil {
			t.Fatal(err)
		}
		j.message(t, j.h.HandleDeleteMe, "/delete_me", "")
		j.callback(t, j.h.HandleConfirmDeleteProfile, j.h.StateManager.Get(42).PendingDeleteToken)
		privacy := repository.NewPrivacyDeletionRepository(j.privacyDB)
		requests, err := privacy.ClaimPending(j.ctx, 10)
		if err != nil || len(requests) != 1 {
			t.Fatalf("deletion request unavailable: %+v %v", requests, err)
		}
		if err := privacy.Complete(j.ctx, requests[0].ID, requests[0].ClaimToken); err != nil {
			t.Fatal(err)
		}
		if user, err := j.h.UserService.GetUser(j.ctx, "42"); err != nil || user != nil {
			t.Fatalf("profile not deleted: %+v %v", user, err)
		}
	})
}

func TestRuntimeInlineRespectsSelectedSubgroup(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	j.callback(t, j.h.HandleSetSubscriptionSubgroup, "journey-a|1|0")
	c := j.telegram.bot.NewContext(tele.Update{Query: &tele.Query{ID: "inline", Sender: &tele.User{ID: 42}, Text: j.date.Format("2006-01-02")}})
	if err := j.h.HandleInlineQuery(c); err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Content struct {
			Text string `json:"message_text"`
		} `json:"input_message_content"`
	}
	last := j.telegram.payloads[len(j.telegram.payloads)-1]
	if err := json.Unmarshal([]byte(last["results"]), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("inline results missing: %s", last["results"])
	}
	text := results[0].Content.Text
	if text == "" {
		t.Fatal("inline article has no input_message_content")
	}
	if !strings.Contains(text, "Предмет подгруппы 1") || !strings.Contains(text, "Предмет подгруппы 0") || strings.Contains(text, "Предмет подгруппы 2") {
		t.Fatalf("inline ignored subgroup: %s", last["results"])
	}
}

func TestRuntimeInlineKeepsLongSchedulesWithinTelegramLimit(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	for i := 0; i < 25; i++ {
		if _, err := j.db.ExecContext(j.ctx, `INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type) VALUES($1,'isuct','journey-term','journey-a',$2,'15:50','17:25','every',$3,'lecture')`, fmt.Sprintf("long-%d", i), weekdayNumber(j.date), strings.Repeat("Расширенное название дисциплины ", 8)); err != nil {
			t.Fatal(err)
		}
	}
	c := j.telegram.bot.NewContext(tele.Update{Query: &tele.Query{ID: "inline", Sender: &tele.User{ID: 42}, Text: j.date.Format("2006-01-02")}})
	if err := j.h.HandleInlineQuery(c); err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Content struct {
			Text string `json:"message_text"`
		} `json:"input_message_content"`
	}
	last := j.telegram.payloads[len(j.telegram.payloads)-1]
	if err := json.Unmarshal([]byte(last["results"]), &results); err != nil || len(results) != 1 {
		t.Fatalf("inline results: %s %v", last["results"], err)
	}
	text := results[0].Content.Text
	if text == "" {
		t.Fatal("inline article has no input_message_content")
	}
	if len([]rune(text)) > tgMaxLen || !strings.Contains(text, "Полное расписание") {
		t.Fatalf("long inline result has %d characters and no usable summary", len([]rune(text)))
	}
}

func TestRuntimeGroupChatSettingsAndSchedule(t *testing.T) {
	j := newRuntimeJourney(t)
	j.telegram.chatRole = "administrator"
	message := func(handler tele.HandlerFunc, text, payload string) {
		t.Helper()
		c := j.telegram.bot.NewContext(tele.Update{Message: &tele.Message{ID: 7, Text: text, Payload: payload, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: -10042, Type: tele.ChatSuperGroup, Title: "Synthetic chat"}}})
		if err := handler(c); err != nil {
			t.Fatal(err)
		}
		j.requireNoError(t)
	}
	callback := func(handler tele.HandlerFunc, args string) {
		t.Helper()
		c := j.telegram.bot.NewContext(tele.Update{Callback: &tele.Callback{ID: "chat-callback", Data: args, Sender: &tele.User{ID: 42}, Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: -10042, Type: tele.ChatSuperGroup, Title: "Synthetic chat"}}}})
		if err := handler(c); err != nil {
			t.Fatal(err)
		}
		j.requireNoError(t)
	}
	message(j.h.HandleSetChatGroup, "/set_chat_group", "isuct 4/147")
	profile, err := j.h.ChatProfileService.Get(j.ctx, "-10042")
	if err != nil || profile == nil || profile.DefaultGroupID != "journey-a" {
		t.Fatalf("chat group unavailable: %+v %v", profile, err)
	}
	for _, handler := range []tele.HandlerFunc{j.h.HandleChatSettings, j.h.HandleToday, j.h.HandleTomorrow, j.h.HandleWeek, j.h.HandleTwoWeeks} {
		message(handler, "schedule", "")
	}
	for _, format := range []string{"compact", "visual"} {
		callback(j.h.HandleSetChatScheduleView, format)
		message(j.h.HandleToday, "/today", "")
	}
	callback(j.h.HandleDownloadSchedule, "ics|"+keyboards.GroupToken("journey-a")+"|"+j.date.Format("2006-01-02")+"|14")
	j.telegram.chatRole = "member"
	message(j.h.HandleSetChatGroup, "/set_chat_group", "isuct 4/245")
	profile, err = j.h.ChatProfileService.Get(j.ctx, "-10042")
	if err != nil || profile == nil || profile.DefaultGroupID != "journey-a" {
		t.Fatal("non-admin changed chat group")
	}
	j.telegram.chatRole = "administrator"
	message(j.h.HandleUnsetChatGroup, "/unset_chat_group", "")
	callback(j.h.HandleConfirmUnsetChatGroup, j.h.StateManager.Get(42).PendingChatUnlinkToken)
	profile, err = j.h.ChatProfileService.Get(j.ctx, "-10042")
	if err != nil || profile != nil {
		t.Fatalf("chat unlink failed: %+v %v", profile, err)
	}
}

func TestRuntimeInlineUnavailableDoesNotResetProfile(t *testing.T) {
	for _, failure := range []string{"inactive_university", "inactive_group", "profile_read_denied", "schedule_read_denied"} {
		t.Run(failure, func(t *testing.T) {
			j := newRuntimeJourney(t)
			j.onboard(t)
			var role string
			if err := j.botDB.GetContext(j.ctx, &role, `SELECT current_user`); err != nil {
				t.Fatal(err)
			}
			query := map[string]string{
				"inactive_university":  `UPDATE universities SET is_active=FALSE WHERE id='isuct'`,
				"inactive_group":       `UPDATE groups SET is_active=FALSE WHERE id='journey-a'`,
				"profile_read_denied":  `REVOKE SELECT ON users FROM ` + pgx.Identifier{role}.Sanitize(),
				"schedule_read_denied": `REVOKE SELECT ON effective_lessons FROM ` + pgx.Identifier{role}.Sanitize(),
			}[failure]
			if _, err := j.db.ExecContext(j.ctx, query); err != nil {
				t.Fatal(err)
			}
			c := j.telegram.bot.NewContext(tele.Update{Query: &tele.Query{ID: "inline", Sender: &tele.User{ID: 42}, Text: "сегодня"}})
			if err := j.h.HandleInlineQuery(c); err != nil {
				t.Fatal(err)
			}
			last := j.telegram.payloads[len(j.telegram.payloads)-1]
			if last["cache_time"] != "1" || last["switch_pm_parameter"] != "menu" || last["results"] != "[]" {
				t.Fatalf("temporary failure treated as setup or fresh data: %+v", last)
			}
			var primary string
			if err := j.db.GetContext(j.ctx, &primary, `SELECT default_group_id FROM users WHERE id='42'`); err != nil || primary != "journey-a" {
				t.Fatalf("profile changed on read failure: %q %v", primary, err)
			}
		})
	}
}

func TestRuntimeInlineSetupAndDateRecovery(t *testing.T) {
	j := newRuntimeJourney(t)
	answer := func(query string) map[string]string {
		t.Helper()
		c := j.telegram.bot.NewContext(tele.Update{Query: &tele.Query{ID: "inline", Sender: &tele.User{ID: 42}, Text: query}})
		if err := j.h.HandleInlineQuery(c); err != nil {
			t.Fatal(err)
		}
		return j.telegram.payloads[len(j.telegram.payloads)-1]
	}
	if result := answer(""); result["cache_time"] != "1" || result["switch_pm_parameter"] != "setup" || result["results"] != "[]" {
		t.Fatalf("missing recoverable setup response: %+v", result)
	}
	j.onboard(t)
	if result := answer("не дата"); result["cache_time"] != "1" || result["switch_pm_parameter"] != "date" || result["results"] != "[]" {
		t.Fatalf("missing recoverable date response: %+v", result)
	}
	result := answer("сегодня")
	if result["results"] == "[]" || !strings.Contains(result["results"], "Предмет подгруппы 0") {
		t.Fatalf("valid query did not recover after setup and invalid date: %+v", result)
	}
}

func TestRuntimeRemoveLastSubscriptionAndChooseAgain(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	j.callback(t, j.h.HandleRequestDeleteSubscription, "journey-a|0")
	j.callback(t, j.h.HandleConfirmDeleteSubscription, j.telegram.button(t, "Да, удалить"))
	user, err := j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user == nil || user.DefaultGroupID != "" {
		t.Fatalf("last subscription not cleared: %+v %v", user, err)
	}
	j.callback(t, j.h.HandleAddSubscription, "0")
	j.message(t, j.h.HandleTextInput, "ИГХТУ 4/245", "")
	j.callback(t, j.h.HandleConfirmPrimaryGroup, "save|"+j.h.StateManager.Get(42).FlowNonce)
	user, err = j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user == nil || user.DefaultGroupID != "journey-b" {
		t.Fatalf("cannot choose a new first subscription: %+v %v", user, err)
	}
}
