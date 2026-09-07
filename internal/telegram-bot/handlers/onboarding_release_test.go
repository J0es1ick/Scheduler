package handlers

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestReleaseOnboardingRequiresExplicitGroupConfirmation(t *testing.T) {
	subscriptions := &subscriptionScenarioService{}
	h := &Handler{StateManager: state.NewManager(), SubscriptionService: subscriptions, GroupService: &inputScenarioGroups{groups: []domain.Group{{ID: "new", Name: "4/147", UniversityID: "isuct", IsActive: true}}}, UniversityService: &navigationUniversityService{universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}}}}
	h.StateManager.Set(42, &dto.UserState{Step: "awaiting_query", UniversityID: "isuct", SetSelectedGroupDefault: true})
	s := newTelegramScenario(t)
	if err := h.HandleTextInput(s.bot.NewContext(tele.Update{Message: &tele.Message{Text: "4/147", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})); err != nil {
		t.Fatal(err)
	}
	if subscriptions.defaultGroup != "" {
		t.Fatal("primary group changed before confirmation")
	}
	if h.StateManager.Get(42).Step != "confirming_primary_group" {
		t.Fatal("confirmation missing")
	}
	s.requireActions(t, "confirm_primary_group")
}
