package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestReportStartOpensDraftForOriginalSchedule(t *testing.T) {
	scenario := newTelegramScenario(t)
	h := &Handler{StateManager: state.NewManager(), UserService: &navigationUserService{user: domain.User{ID: "42"}}, SubscriptionService: &subscriptionScenarioService{items: []domain.GroupSubscription{{GroupID: "other-group", GroupName: "4/147", UniversityID: "isuct", UniversityName: "ИГХТУ", Subgroup: 1, IsActive: true}}}}
	target := &scheduleTarget{GroupID: "other-group", Subgroup: 2}
	payload := "report_" + target.navigationReference() + "_20260912_2"
	c := scenario.bot.NewContext(tele.Update{Message: &tele.Message{Text: "/start " + payload, Payload: payload, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42}}})
	if err := h.HandleStart(c); err != nil {
		t.Fatal(err)
	}
	draft := h.StateManager.Get(42)
	if draft == nil || draft.Step != "awaiting_hotline_submission" || draft.HotlineType != domain.SupportRequestUpdateExisting {
		t.Fatalf("report draft=%+v", draft)
	}
	for _, part := range []string{"4/147", "12.09.2026", "Подгруппа: 2"} {
		if !strings.Contains(draft.HotlineContext, part) {
			t.Errorf("lost original context %s: %s", part, draft.HotlineContext)
		}
	}
	if len(scenario.methods) != 1 || scenario.methods[0] != "sendMessage" {
		t.Fatalf("unexpected side effects: %v", scenario.methods)
	}
}

func TestReportFooterUsesNativeCommand(t *testing.T) {
	bot, err := tele.NewBot(tele.Settings{Token: "test", Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, username := range []string{"", "scheduler_test_bot"} {
		bot.Me = &tele.User{Username: username}
		c := bot.NewContext(tele.Update{Message: &tele.Message{Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
		if footer := scheduleReportLink(c); footer != "\nСообщить об ошибке: /report" {
			t.Fatalf("footer must leave /report outside a URL or formatting entity: %q", footer)
		}
	}
	c := bot.NewContext(tele.Update{Message: &tele.Message{Chat: &tele.Chat{ID: -42, Type: tele.ChatSuperGroup}}})
	if footer := scheduleReportLink(c); footer != "" {
		t.Fatalf("private report command in group: %q", footer)
	}
}

func TestReportCommandOpensDraftWithoutStartOrSubmission(t *testing.T) {
	scenario := newTelegramScenario(t)
	h := &Handler{StateManager: state.NewManager(), UserService: &navigationUserService{user: domain.User{ID: "42"}}}
	scenario.bot.Handle("/report", h.PrivateOnly(h.HandleReport))
	scenario.bot.Handle("/start", func(tele.Context) error {
		t.Error("report opened /start")
		return nil
	})
	scenario.bot.ProcessUpdate(tele.Update{Message: &tele.Message{Text: "/report", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42}}})
	draft := h.StateManager.Get(42)
	if draft == nil || draft.Step != "awaiting_hotline_submission" || draft.HotlineType != domain.SupportRequestUpdateExisting || draft.HotlineContext != "" {
		t.Fatalf("report draft=%+v", draft)
	}
	if len(scenario.methods) != 1 || scenario.methods[0] != "sendMessage" {
		t.Fatalf("opening a draft must not submit or open a menu: %v", scenario.methods)
	}
}

func TestLegacyReportPayloadPreservesTargetDateAndSubgroup(t *testing.T) {
	target := &scheduleTarget{GroupID: "other-group", Subgroup: 2}
	date := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	payload := "report_" + target.navigationReference() + "_20260912_2"
	ref, parsed, sub, err := parseReportPayload(payload)
	if err != nil || ref != target.navigationReference() || !parsed.Equal(date) || sub != 2 {
		t.Fatalf("parsed=%s %v %d %v", ref, parsed, sub, err)
	}
	for _, invalid := range []string{"report_bad_20260912_2", "report_1234567890abcdef_20260230_0", "report_1234567890abcdef_20260912_-1", "report_1234567890abcdef_20260912_2_extra"} {
		if _, _, _, err := parseReportPayload(invalid); err == nil {
			t.Errorf("accepted %s", invalid)
		}
	}
}
