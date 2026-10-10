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

type personalReminderProvider struct{ staticScheduleProvider }

func (p personalReminderProvider) PersonalizeSchedule(_ context.Context, userID, _, _, _ string, data map[time.Time][]domain.Lesson) (map[time.Time][]domain.Lesson, error) {
	result := map[time.Time][]domain.Lesson{}
	for date, lessons := range data {
		if userID == "cancelled" {
			result[date] = nil
			continue
		}
		result[date] = append([]domain.Lesson(nil), lessons...)
		if userID == "edited" {
			for i := range result[date] {
				result[date][i].Room = "Personal room"
			}
		}
	}
	return result, nil
}
func TestPersonalReminderCacheAndLastMinuteCancellation(t *testing.T) {
	provider := personalReminderProvider{staticScheduleProvider{lessons: []domain.Lesson{{Subject: "Physics", Room: "Original", TimeStart: "09:00", TimeEnd: "10:30"}}}}
	repo := &fakeReminderRepository{}
	w := ReminderWorker{repository: repo, scheduleService: provider}
	now := time.Date(2026, 10, 5, 8, 50, 0, 0, time.UTC)
	cache := make(reminderScheduleCache)
	for _, user := range []string{"cancelled", "edited", "ordinary"} {
		recipient := domain.ReminderRecipient{UserID: user, GroupID: "group", GroupName: "Group", Timezone: "UTC", ReminderMinutes: 15}
		if err := w.enqueueRecipientReminders(context.Background(), recipient, now, cache); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.enqueued) != 2 || repo.enqueued[0].userID != "edited" || !strings.Contains(repo.enqueued[0].body, "Personal room") || !strings.Contains(repo.enqueued[1].body, "Original") {
		t.Fatalf("personal cache isolation: %+v", repo.enqueued)
	}
	starts := now.Add(10 * time.Minute)
	raw, _ := json.Marshal(domain.ReminderContext{Date: "2026-10-05", TimeStart: "09:00", TimeEnd: "10:30", StartsAt: starts})
	item := domain.BotOutboxDelivery{UserID: "cancelled", GroupID: "group", ExpiresAt: &starts, ReminderContext: raw}
	delivery := NotificationWorker{reminderSchedule: provider, reminderRecipient: func(context.Context, string, string) (*domain.ReminderRecipient, error) {
		return &domain.ReminderRecipient{UserID: "cancelled", GroupID: "group", Timezone: "UTC", ReminderMinutes: 15}, nil
	}}
	if _, send, err := delivery.refreshReminder(context.Background(), item, now); err != nil || send {
		t.Fatalf("cancelled personal lesson sent: %t %v", send, err)
	}
}
