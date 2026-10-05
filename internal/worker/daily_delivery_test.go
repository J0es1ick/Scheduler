package worker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

type dailyProgressStub struct {
	notificationRepositoryStub
	messages  []domain.ScheduleMessage
	delivered int
}

func (r *dailyProgressStub) SaveScheduleMessages(_ context.Context, _, _ string, messages []domain.ScheduleMessage) error {
	r.messages = messages
	return nil
}
func (r *dailyProgressStub) SaveDeliveredParts(_ context.Context, _, _ string, count int) error {
	r.delivered = count
	return nil
}

func TestDailyDeliveryResumesAfterConfirmedParts(t *testing.T) {
	repo := &dailyProgressStub{}
	built := 0
	fail := true
	var sent []string
	w := &NotificationWorker{repository: repo, limiter: telegramlimit.New(0, 0), waitSend: func(context.Context, string) error { return nil },
		dailySchedule: func(context.Context, string, time.Time) ([]domain.ScheduleMessage, error) {
			built++
			return []domain.ScheduleMessage{{Text: "one"}, {Text: "two"}, {Text: "three"}}, nil
		},
		bot: notificationSenderFunc(func(_ tele.Recipient, body interface{}, _ ...interface{}) (*tele.Message, error) {
			if body == "two" && fail {
				fail = false
				return nil, errors.New("temporary Telegram failure")
			}
			sent = append(sent, body.(string))
			return &tele.Message{}, nil
		})}
	item := domain.BotOutboxDelivery{UserID: "42", ID: "daily", ClaimToken: "claim", ScheduleContext: json.RawMessage(`{"date":"2026-10-04","timezone":"Europe/Moscow"}`), ScheduleMessages: json.RawMessage(`[]`)}
	if err := w.sendDailySchedule(context.Background(), item); err == nil {
		t.Fatal("expected send failure")
	}
	if repo.delivered != 1 {
		t.Fatalf("progress %d", repo.delivered)
	}
	item.DeliveredParts = repo.delivered
	item.ScheduleMessages, _ = json.Marshal(repo.messages)
	if err := w.sendDailySchedule(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if built != 1 || repo.delivered != 3 || !reflect.DeepEqual(sent, []string{"one", "two", "three"}) {
		t.Fatalf("built=%d progress=%d sent=%v", built, repo.delivered, sent)
	}
}

func TestDailyDeliveryChecksProfileBeforeEachPart(t *testing.T) {
	repo := &dailyProgressStub{}
	sent := 0
	repo.active = func(context.Context, string, string) (bool, error) { return sent == 0, nil }
	w := &NotificationWorker{repository: repo, limiter: telegramlimit.New(0, 0), waitSend: func(context.Context, string) error { return nil },
		dailySchedule: func(context.Context, string, time.Time) ([]domain.ScheduleMessage, error) {
			return []domain.ScheduleMessage{{Text: "one"}, {Text: "two"}}, nil
		},
		bot: notificationSenderFunc(func(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error) {
			sent++
			return &tele.Message{}, nil
		})}
	item := domain.BotOutboxDelivery{UserID: "42", ScheduleContext: json.RawMessage(`{"date":"2026-10-04","timezone":"Europe/Moscow"}`), ScheduleMessages: json.RawMessage(`[]`)}
	if err := w.sendDailySchedule(context.Background(), item); !errors.Is(err, errDailyCancelled) {
		t.Fatalf("cancellation %v", err)
	}
	if sent != 1 || repo.delivered != 1 {
		t.Fatalf("sent=%d progress=%d", sent, repo.delivered)
	}
}

type teacherReminderSchedule struct {
	emptyScheduleProvider
	university, name string
}

func (s *teacherReminderSchedule) GetScheduleForTeacher(_ context.Context, university, name string, _ time.Time) ([]domain.Lesson, error) {
	s.university = university
	s.name = name
	return []domain.Lesson{{TimeStart: "09:00", TimeEnd: "10:00", Subject: "Math", GroupName: "Group A"}}, nil
}
func TestTeacherRemindersUsePersonalIdentityAndGroupNames(t *testing.T) {
	repo := &fakeReminderRepository{}
	provider := &teacherReminderSchedule{}
	w := &ReminderWorker{repository: repo, scheduleService: provider}
	recipient := domain.ReminderRecipient{UserID: "42", TeacherID: "t", UniversityID: "u", GroupName: "Иванов И.И.", Timezone: "Europe/Moscow", ReminderMinutes: 15}
	location, _ := time.LoadLocation(recipient.Timezone)
	if err := w.enqueueRecipientReminders(context.Background(), recipient, time.Date(2026, 10, 4, 8, 50, 0, 0, location), reminderScheduleCache{}); err != nil {
		t.Fatal(err)
	}
	if provider.university != "u" || provider.name != recipient.GroupName || len(repo.enqueued) != 1 {
		t.Fatalf("provider %#v queued %#v", provider, repo.enqueued)
	}
}
