package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

type fakeReminderRepository struct {
	recipients []domain.ReminderRecipient
	calls      []string
	blockOnce  bool
	enqueued   []enqueuedReminder
}

type enqueuedReminder struct {
	userID string
	body   string
}

func (r *fakeReminderRepository) ActiveRecipientsPage(
	ctx context.Context,
	after string,
	limit int,
) ([]domain.ReminderRecipient, error) {
	r.calls = append(r.calls, after)
	if r.blockOnce && len(r.calls) == 2 {
		r.blockOnce = false
		<-ctx.Done()
		return nil, ctx.Err()
	}
	result := make([]domain.ReminderRecipient, 0, limit)
	for _, recipient := range r.recipients {
		if recipient.UserID > after {
			result = append(result, recipient)
		}
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (r *fakeReminderRepository) Enqueue(
	_ context.Context,
	_ string,
	userID string,
	_ string,
	body string,
) error {
	r.enqueued = append(r.enqueued, enqueuedReminder{userID: userID, body: body})
	return nil
}

type fakeWorkerStatusRepository struct {
	status domain.WorkerStatus
	runs   []domain.WorkerRunResult
}

func (r *fakeWorkerStatusRepository) Get(
	context.Context,
	string,
) (*domain.WorkerStatus, error) {
	result := r.status
	return &result, nil
}

func (r *fakeWorkerStatusRepository) RecordRun(
	_ context.Context,
	result domain.WorkerRunResult,
) error {
	r.runs = append(r.runs, result)
	r.status.Cursor = result.Cursor
	return nil
}

type emptyScheduleProvider struct{}

func (emptyScheduleProvider) GetScheduleForGroup(
	context.Context,
	string,
	time.Time,
) ([]domain.Lesson, error) {
	return []domain.Lesson{}, nil
}

type recordingScheduleProvider struct {
	dates []time.Time
}

func (p *recordingScheduleProvider) GetScheduleForGroup(
	_ context.Context,
	_ string,
	date time.Time,
) ([]domain.Lesson, error) {
	p.dates = append(p.dates, date)
	return []domain.Lesson{}, nil
}

type staticScheduleProvider struct {
	lessons []domain.Lesson
}

func (p staticScheduleProvider) GetScheduleForGroup(
	context.Context,
	string,
	time.Time,
) ([]domain.Lesson, error) {
	return p.lessons, nil
}

func TestReminderSlotsCombineConcurrentSubgroups(t *testing.T) {
	lessons := []domain.Lesson{
		{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Математика", Subgroup: 1},
		{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Математика", Subgroup: 2},
		{TimeStart: "12:10", TimeEnd: "13:45", Subject: "Физика"},
	}
	slots := reminderSlots(lessons)
	if len(slots) != 2 {
		t.Fatalf("slots = %d, want 2", len(slots))
	}
	if len(slots[0].Lessons) != 2 {
		t.Fatalf("first slot lessons = %d, want 2", len(slots[0].Lessons))
	}
}

func TestReminderIDIsStableForSameTimeslot(t *testing.T) {
	date := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	first := reminderID("42", "group", date, "09:50", "11:25")
	second := reminderID("42", "group", date, "09:50", "11:25")
	if first != second {
		t.Fatalf("reminder ids differ: %q and %q", first, second)
	}
}

func TestReminderWorkerUsesUniversityTimezone(t *testing.T) {
	provider := &recordingScheduleProvider{}
	worker := &ReminderWorker{
		repository:      &fakeReminderRepository{},
		scheduleService: provider,
	}
	recipient := domain.ReminderRecipient{
		UserID: "42", GroupID: "group", Timezone: "Asia/Yekaterinburg",
	}
	nowUTC := time.Date(2026, time.January, 1, 20, 30, 0, 0, time.UTC)
	if err := worker.enqueueRecipientReminders(
		context.Background(), recipient, nowUTC, make(reminderScheduleCache),
	); err != nil {
		t.Fatal(err)
	}
	if len(provider.dates) == 0 || provider.dates[0].Format("2006-01-02") != "2026-01-02" {
		t.Fatalf("first local date = %v, want 2026-01-02", provider.dates)
	}
	if provider.dates[0].Location().String() != "Asia/Yekaterinburg" {
		t.Fatalf("location = %s", provider.dates[0].Location())
	}
}

func TestReminderWorkerDoesNotMutateCachedScheduleBetweenSubgroups(t *testing.T) {
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeReminderRepository{}
	worker := &ReminderWorker{
		repository: repository,
		scheduleService: staticScheduleProvider{lessons: []domain.Lesson{
			{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Подгруппа 2", Subgroup: 2},
			{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Общая пара"},
			{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Подгруппа 1", Subgroup: 1},
		}},
	}
	now := time.Date(2026, time.September, 1, 9, 40, 0, 0, location)
	schedules := make(reminderScheduleCache)
	for _, recipient := range []domain.ReminderRecipient{
		{UserID: "subgroup-1", GroupID: "group", GroupName: "G", UniversityName: "U", Subgroup: 1, ReminderMinutes: 20, Timezone: "Europe/Moscow"},
		{UserID: "subgroup-2", GroupID: "group", GroupName: "G", UniversityName: "U", Subgroup: 2, ReminderMinutes: 20, Timezone: "Europe/Moscow"},
	} {
		if err := worker.enqueueRecipientReminders(context.Background(), recipient, now, schedules); err != nil {
			t.Fatal(err)
		}
	}
	if len(repository.enqueued) != 2 {
		t.Fatalf("enqueued reminders=%d, want 2", len(repository.enqueued))
	}
	for _, reminder := range repository.enqueued {
		if !strings.Contains(reminder.body, "Общая пара") {
			t.Fatalf("common lesson missing for %s: %s", reminder.userID, reminder.body)
		}
		if reminder.userID == "subgroup-1" {
			if !strings.Contains(reminder.body, "Подгруппа 1") || strings.Contains(reminder.body, "Подгруппа 2") {
				t.Fatalf("wrong subgroup 1 reminder: %s", reminder.body)
			}
		} else if !strings.Contains(reminder.body, "Подгруппа 2") || strings.Contains(reminder.body, "Подгруппа 1") {
			t.Fatalf("wrong subgroup 2 reminder: %s", reminder.body)
		}
	}
}

func TestReminderTextIncludesEveryConcurrentLesson(t *testing.T) {
	text := reminderText(
		domain.ReminderRecipient{
			UniversityName: "ИГХТУ",
			GroupName:      "3/42",
		},
		time.Date(2026, time.July, 27, 0, 0, 0, 0, time.Local),
		reminderSlot{
			TimeStart: "09:50",
			TimeEnd:   "11:25",
			Lessons: []domain.Lesson{
				{Subject: "Математика", Subgroup: 1, Room: "А206"},
				{Subject: "Физика", Subgroup: 2, Room: "А208"},
			},
		},
		14*time.Minute+time.Second,
	)
	for _, expected := range []string{
		"через 15 мин.",
		"Математика",
		"Физика",
		"подгруппа 1",
		"/reminders",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("reminder text does not contain %q:\n%s", expected, text)
		}
	}
}

func TestReminderWorkerResumesFromStoredCursor(t *testing.T) {
	recipients := &fakeReminderRepository{
		recipients: []domain.ReminderRecipient{{UserID: "200", GroupID: "group", Timezone: "Europe/Moscow"}},
	}
	statuses := &fakeWorkerStatusRepository{
		status: domain.WorkerStatus{Cursor: "100"},
	}
	worker := &ReminderWorker{
		repository:       recipients,
		statusRepository: statuses,
		scheduleService:  emptyScheduleProvider{},
		runTimeout:       time.Second,
		now:              time.Now,
	}

	worker.tick(context.Background())

	if len(recipients.calls) == 0 || recipients.calls[0] != "100" {
		t.Fatalf("first page cursor = %q, want 100", recipients.calls[0])
	}
	if len(statuses.runs) != 1 {
		t.Fatalf("recorded runs = %d, want 1", len(statuses.runs))
	}
	run := statuses.runs[0]
	if run.Cursor != "" || run.LastFullCycleAt == nil || run.Processed != 1 {
		t.Fatalf("unexpected completed run: %+v", run)
	}
}

func TestReminderWorkerContinuesAfterTimedOutPage(t *testing.T) {
	items := make([]domain.ReminderRecipient, 0, reminderBatchSize)
	for index := 0; index < reminderBatchSize; index++ {
		items = append(items, domain.ReminderRecipient{
			UserID:   fmt.Sprintf("%03d", index),
			GroupID:  "group",
			Timezone: "Europe/Moscow",
		})
	}
	recipients := &fakeReminderRepository{recipients: items, blockOnce: true}
	statuses := &fakeWorkerStatusRepository{}
	worker := &ReminderWorker{
		repository:       recipients,
		statusRepository: statuses,
		scheduleService:  emptyScheduleProvider{},
		runTimeout:       20 * time.Millisecond,
		now:              time.Now,
	}

	worker.tick(context.Background())
	if len(statuses.runs) != 1 {
		t.Fatalf("recorded runs after timeout = %d, want 1", len(statuses.runs))
	}
	firstRun := statuses.runs[0]
	if firstRun.Cursor != "249" || firstRun.Processed != reminderBatchSize || firstRun.LastError == "" {
		t.Fatalf("unexpected timed out run: %+v", firstRun)
	}

	worker.tick(context.Background())
	if len(recipients.calls) < 3 || recipients.calls[2] != "249" {
		t.Fatalf("resumed cursor calls = %v, want third call from 249", recipients.calls)
	}
	secondRun := statuses.runs[1]
	if secondRun.Cursor != "" || secondRun.LastFullCycleAt == nil {
		t.Fatalf("unexpected resumed run: %+v", secondRun)
	}
}
