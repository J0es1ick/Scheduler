package managedparser

import (
	"context"
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
)

type externalIdentityParser struct{}

func (externalIdentityParser) Manifest() managed.Manifest { return managed.Manifest{} }

func (externalIdentityParser) FetchGroups(context.Context) ([]managed.Group, error) {
	return []managed.Group{{ExternalID: "source-group-101", Name: "1-ЭЭ-В"}}, nil
}

func (externalIdentityParser) FetchSchedule(context.Context, string) ([]managed.Lesson, error) {
	return nil, nil
}

func TestFetchGroupsKeepsManagedExternalIdentity(t *testing.T) {
	adapter := &adapter{
		parser: externalIdentityParser{},
		manifest: managed.Manifest{
			ParserID: "test-parser",
			Institution: connector.Institution{
				ExternalID: "test-university",
			},
		},
		groups: make(map[string]managed.Group),
	}
	groups, err := adapter.FetchGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ExternalID != "source-group-101" {
		t.Fatalf("managed group external identity = %+v", groups)
	}
}

func TestConvertLessonKeepsManagedParityContract(t *testing.T) {
	adapter := &adapter{
		manifest: managed.Manifest{
			ParserID: "test-parser",
			Institution: connector.Institution{
				ExternalID: "test-university",
			},
		},
	}
	input := managed.Lesson{
		ExternalID: "lesson-1",
		Subject:    "Математика",
		Schedule: connector.Schedule{
			DayOfWeek: 2,
			StartsAt:  "09:50",
			EndsAt:    "11:25",
			Recurrence: connector.Recurrence{
				Kind:      connector.RecurrenceEven,
				ValidFrom: "2026-09-01",
				ValidTo:   "2026-12-30",
			},
		},
	}

	lesson, err := convertLesson(adapter, "group-1", input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if lesson.Recurrence.CycleLength != 2 || len(lesson.Recurrence.CycleWeeks) != 1 ||
		lesson.Recurrence.CycleWeeks[0] != 2 || lesson.Recurrence.AnchorDate != nil {
		t.Fatalf("unexpected managed recurrence: %#v", lesson.Recurrence)
	}
}
