//go:build integration

package handlers

import (
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
	"strings"
	"testing"
)

func TestRuntimeGroupInvitationConfirmsSubgroupAndPreservesConsent(t *testing.T) {
	j := newRuntimeJourney(t)
	j.telegram.bot.Me = &tele.User{ID: 1, Username: "scheduler_test_bot"}
	invite := "group_" + keyboards.GroupToken("journey-a")
	j.message(t, j.h.HandleStart, "/start", invite)
	user, err := j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user.DefaultGroupID != "" || user.NotificationsEnabled {
		t.Fatalf("invitation mutated profile before confirmation: %+v %v", user, err)
	}
	j.callback(t, j.h.HandleInvitationConfirm, "primary|"+j.h.StateManager.Get(42).FlowNonce)
	savedButton := j.telegram.button(t, "Подгруппа 2")
	j.callback(t, j.h.HandleInvitationSubgroup, savedButton)
	user, err = j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user.DefaultGroupID != "journey-a" || user.NotificationsEnabled || user.DailyEnabled || user.ReminderEnabled || user.ServiceUpdatesConsent != nil || user.DailySetup != "done" {
		t.Fatalf("consent changed: %+v %v", user, err)
	}
	groups, err := j.h.SubscriptionService.GetGroupSubscriptions(j.ctx, "42")
	if err != nil || len(groups) != 1 || groups[0].Subgroup != 2 {
		t.Fatalf("subgroup: %+v %v", groups, err)
	}
	j.callback(t, j.h.HandleInvitationSubgroup, savedButton)
	groups, err = j.h.SubscriptionService.GetGroupSubscriptions(j.ctx, "42")
	if err != nil || len(groups) != 1 {
		t.Fatal("repeated invitation duplicated subscription", err)
	}
	j.message(t, j.h.HandleStart, "/start", "group_"+keyboards.GroupToken("journey-b"))
	j.callback(t, j.h.HandleInvitationConfirm, "cancel|"+j.h.StateManager.Get(42).FlowNonce)
	groups, err = j.h.SubscriptionService.GetGroupSubscriptions(j.ctx, "42")
	if err != nil || len(groups) != 1 {
		t.Fatal("cancel subscribed", err)
	}
	j.message(t, j.h.HandleStart, "/start", "group_"+keyboards.GroupToken("journey-b"))
	j.callback(t, j.h.HandleInvitationConfirm, "additional|"+j.h.StateManager.Get(42).FlowNonce)
	j.callback(t, j.h.HandleInvitationSubgroup, "0|"+j.h.StateManager.Get(42).FlowNonce)
	user, err = j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user.DefaultGroupID != "journey-a" {
		t.Fatal("additional invitation replaced primary", err)
	}
	j.message(t, j.h.HandleShareGroup, "/share", "")
	if !strings.Contains(j.telegram.text, "https://t.me/scheduler_test_bot?start="+invite) {
		t.Fatal("share URL missing", j.telegram.text)
	}
	if _, err = j.db.ExecContext(j.ctx, `UPDATE groups SET is_active=FALSE WHERE id='journey-b'`); err != nil {
		t.Fatal(err)
	}
	j.message(t, j.h.HandleStart, "/start", "group_"+keyboards.GroupToken("journey-b"))
	if !strings.Contains(j.telegram.text, "недоступна") {
		t.Fatal("inactive invitation has no explanation", j.telegram.text)
	}
}
