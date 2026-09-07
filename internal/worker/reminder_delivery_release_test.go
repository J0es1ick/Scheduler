package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestReminderUsesLiveSlotAndExpiresAtStart(t *testing.T) {
	starts := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	slot := domain.ReminderContext{Date: "2026-09-02", TimeStart: "09:00", TimeEnd: "10:30", StartsAt: starts, Subgroup: 1}
	raw, _ := json.Marshal(slot)
	item := domain.BotOutboxDelivery{UserID: "123", GroupID: "group", Body: "Old room 100", ExpiresAt: &starts, ReminderContext: raw}
	recipient := &domain.ReminderRecipient{UserID: "123", GroupID: "group", Timezone: "UTC", Subgroup: 1, ReminderMinutes: 15}
	worker := NotificationWorker{reminderSchedule: staticScheduleProvider{lessons: []domain.Lesson{{Subject: "Physics", Room: "NEW-200", TimeStart: "09:00", TimeEnd: "10:30", Subgroup: 1}}}, reminderRecipient: func(context.Context, string, string) (*domain.ReminderRecipient, error) { return recipient, nil }}
	body, send, err := worker.refreshReminder(context.Background(), item, starts.Add(-time.Minute))
	if err != nil || !send || !strings.Contains(body, "NEW-200") || strings.Contains(body, "Old room") {
		t.Fatalf("live reminder=%q send=%t err=%v", body, send, err)
	}
	recipient.ReminderMinutes = 5
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-10*time.Minute)); err != nil || send {
		t.Fatal("reminder ignored the reduced reminder interval")
	}
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-5*time.Minute)); err != nil || !send {
		t.Fatal("reminder was not eligible at the new interval")
	}
	for _, now := range []time.Time{starts, starts.Add(time.Hour)} {
		if _, send, err = worker.refreshReminder(context.Background(), item, now); err != nil || send {
			t.Fatalf("expired reminder send=%t err=%v", send, err)
		}
	}
	worker.reminderSchedule = emptyScheduleProvider{}
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-time.Minute)); err != nil || send {
		t.Fatal("cancelled lesson still sends reminder")
	}
	worker.reminderSchedule = staticScheduleProvider{lessons: []domain.Lesson{{TimeStart: "09:00", TimeEnd: "10:30", Subgroup: 1}}}
	recipient.Subgroup = 2
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-time.Minute)); err != nil || send {
		t.Fatal("changed subgroup still sends reminder")
	}
	recipient = nil
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-time.Minute)); err != nil || send {
		t.Fatal("disabled reminder still sends")
	}
	item.ExpiresAt = nil
	if _, send, err = worker.refreshReminder(context.Background(), item, starts.Add(-time.Minute)); err != nil || send {
		t.Fatal("legacy reminder still sends")
	}
}
