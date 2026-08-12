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
