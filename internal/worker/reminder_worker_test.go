package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

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
