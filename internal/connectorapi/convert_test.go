package connectorapi

import (
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestConvertCycleSnapshot(t *testing.T) {
	input := connector.Snapshot{
		SchemaVersion: connector.SchemaVersion, SnapshotID: "one", GeneratedAt: time.Now(),
		Institution: connector.Institution{ExternalID: "uni", Name: "Uni", Timezone: "Europe/Moscow"},
		Term:        connector.Term{ExternalID: "autumn", Name: "Autumn", StartsOn: "2026-09-01", EndsOn: "2027-01-31"},
		Groups: []connector.Group{{ExternalID: "g1", Name: "G1", Lessons: []connector.Lesson{{
			ExternalID: "l1", Subject: "Math", Type: "lecture",
			Schedule: connector.Schedule{DayOfWeek: 1, StartsAt: "09:00", EndsAt: "10:30", Recurrence: connector.Recurrence{
				Kind: connector.RecurrenceCycle, CycleLength: 4, CycleWeeks: []int{1, 3},
			}},
		}}}},
	}
	result, err := convertSnapshot("source", "uni", input)
	if err != nil {
		t.Fatal(err)
	}
	lesson := result.Groups[0].Lessons[0]
	if lesson.WeekType != domain.WeekTypeEvery || lesson.Recurrence.CycleLength != 4 {
		t.Fatalf("unexpected recurrence: %#v", lesson)
	}
}

func TestConvertAlternatingWeeksUsesTermAnchor(t *testing.T) {
	input := connector.Snapshot{
		SchemaVersion: connector.SchemaVersion, SnapshotID: "parity", GeneratedAt: time.Now(),
		Institution: connector.Institution{ExternalID: "uni", Name: "Uni", Timezone: "Europe/Moscow"},
		Term:        connector.Term{ExternalID: "autumn", Name: "Autumn", StartsOn: "2026-08-31", EndsOn: "2027-01-31"},
		Groups: []connector.Group{{ExternalID: "g1", Name: "G1", Lessons: []connector.Lesson{
			{ExternalID: "odd", Subject: "Odd", Type: "lecture", Schedule: connector.Schedule{
				DayOfWeek: 2, StartsAt: "09:00", EndsAt: "10:30", Recurrence: connector.Recurrence{Kind: connector.RecurrenceOdd},
			}},
			{ExternalID: "even", Subject: "Even", Type: "lecture", Schedule: connector.Schedule{
				DayOfWeek: 2, StartsAt: "09:00", EndsAt: "10:30", Recurrence: connector.Recurrence{Kind: connector.RecurrenceEven},
			}},
		}}},
	}
	result, err := convertSnapshot("source", "uni", input)
	if err != nil {
		t.Fatal(err)
	}
	odd := result.Groups[0].Lessons[0].Recurrence
	even := result.Groups[0].Lessons[1].Recurrence
	if odd.CycleLength != 2 || odd.CycleWeeks[0] != 1 || odd.AnchorDate == nil {
		t.Fatalf("unexpected odd recurrence: %#v", odd)
	}
	if even.CycleLength != 2 || even.CycleWeeks[0] != 2 || even.AnchorDate == nil {
		t.Fatalf("unexpected even recurrence: %#v", even)
	}
}
