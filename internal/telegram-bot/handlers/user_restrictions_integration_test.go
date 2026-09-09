//go:build integration

package handlers

import (
	"errors"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/admin"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	tele "gopkg.in/telebot.v3"
)

func TestRuntimeUserRestrictionsSupportAndDelivery(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	store := admin.NewStore(j.adminDB)
	users := repository.NewUserRepository(j.botDB)
	actor := admin.AdminIdentity{ID: "43", Name: "Synthetic owner", Role: "owner"}
	change := func(botBlocked, supportBlocked bool) {
		t.Helper()
		want := domain.UserRestrictions{BotBlocked: botBlocked, SupportBlocked: supportBlocked}
		got, err := store.UpdateUserRestrictions(j.ctx, "42", admin.UserRestrictionsPatch{BotBlocked: &botBlocked, SupportBlocked: &supportBlocked}, actor, "")
		if err != nil || got != want {
			t.Fatalf("change=%+v %v", got, err)
		}
		got, err = users.Restrictions(j.ctx, "42")
		if err != nil || got != want {
			t.Fatalf("runtime read=%+v %v", got, err)
		}
		profile, err := users.GetUserByID(j.ctx, "42")
		if err != nil || profile.UserRestrictions != want {
			t.Fatalf("profile=%+v %v", profile, err)
		}
	}
	for _, column := range []string{"bot_blocked", "support_blocked"} {
		if _, err := j.botDB.Exec(`UPDATE users SET ` + column + `=TRUE WHERE id='42'`); err == nil {
			t.Fatalf("bot role can set %s", column)
		}
	}
	change(false, true)
	support := repository.NewSupportRequestRepository(j.botDB)
	if err := support.Create(j.ctx, "blocked-request", "42", domain.SupportRequestFeedback, "This feedback must never be accepted"); !errors.Is(err, repository.ErrSupportRequestBlocked) {
		t.Fatalf("support restriction: %v", err)
	}
	j.h.StateManager.Set(42, &dto.UserState{Step: "awaiting_hotline_submission", HotlineType: domain.SupportRequestFeedback})
	var sent int
	c := restrictionSubmissionContext{Context: j.telegram.bot.NewContext(tele.Update{Message: &tele.Message{Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}}), sent: &sent}
	if err := j.h.HandleHotlineSubmission(c, "Already opened form must not bypass a new restriction"); err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatal("blocked submission received a reply")
	}
	var count int
	if err := j.db.Get(&count, `SELECT count(*) FROM support_requests WHERE user_id='42'`); err != nil || count != 0 {
		t.Fatalf("support rows=%d %v", count, err)
	}
	if _, err := j.db.Exec(`UPDATE users SET reminder_enabled=TRUE WHERE id='42';
		INSERT INTO schedule_change_events(id,group_id,source,summary) VALUES('restricted-event','journey-a','manual','Synthetic change');
		INSERT INTO notification_deliveries(id,event_id,user_id) VALUES('restricted-delivery','restricted-event','42');
		INSERT INTO bot_outbox(id,user_id,kind,body) VALUES('restricted-outbox','42','support_resolution','Synthetic reply');`); err != nil {
		t.Fatal(err)
	}
	queue := repository.NewNotificationRepository(j.botDB)
	deliveries, err := queue.ClaimPending(j.ctx, 10)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("support mute affected schedules: %+v %v", deliveries, err)
	}
	outbox, err := queue.ClaimBotOutbox(j.ctx, 10)
	if err != nil || len(outbox) != 1 {
		t.Fatalf("support mute affected outgoing reply: %+v %v", outbox, err)
	}
	change(true, true)
	decision, err := queue.DeliveryDecision(j.ctx, deliveries[0].ID, deliveries[0].ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("claimed schedule not blocked: %s %v", decision, err)
	}
	decision, err = queue.BotOutboxDecision(j.ctx, outbox[0].ID, outbox[0].ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("claimed outbox not blocked: %s %v", decision, err)
	}
	recipients, err := repository.NewReminderRepository(j.botDB).ActiveRecipientsPage(j.ctx, "", 100)
	if err != nil || len(recipients) != 0 {
		t.Fatalf("blocked reminders still generated: %+v %v", recipients, err)
	}
	if _, err = queue.ClaimPending(j.ctx, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = queue.ClaimBotOutbox(j.ctx, 10); err != nil {
		t.Fatal(err)
	}
	change(false, false)
	if err = support.Create(j.ctx, "unblocked-request", "42", domain.SupportRequestFeedback, "Feedback works immediately after unblocking"); err != nil {
		t.Fatal(err)
	}
	recipients, err = repository.NewReminderRepository(j.botDB).ActiveRecipientsPage(j.ctx, "", 100)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("reminders did not resume: %+v %v", recipients, err)
	}
	if err = j.db.Get(&count, `SELECT count(*) FROM admin_audit_logs WHERE action='update_user_restrictions' AND object_id='42'`); err != nil || count != 3 {
		t.Fatalf("audit count=%d %v", count, err)
	}
}

type restrictionSubmissionContext struct {
	tele.Context
	sent *int
}

func (c restrictionSubmissionContext) Send(interface{}, ...interface{}) error { *c.sent++; return nil }
