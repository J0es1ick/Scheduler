package ivgpu

import (
	"context"
	"os"
	"testing"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
)

func TestManifest(t *testing.T) {
	if err := managed.ValidateManifest(Manifest()); err != nil {
		t.Fatal(err)
	}
}

func TestLessonTimeAndTypeMapping(t *testing.T) {
	start, end, err := lessonTime(map[string]string{"-1": "8:30 - 20:50"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if start != "08:30" || end != "20:50" {
		t.Fatalf("time = %s-%s", start, end)
	}
	cases := map[string]string{"ЛЕК": "lecture", "ПР. ЗАН.": "practice", "Лабораторная": "lab", "ЭКЗАМЕН": "exam", "зачёт": "credit", "Консультация": "consultation"}
	for input, expected := range cases {
		if actual := lessonType(input); actual != expected {
			t.Errorf("lessonType(%q)=%q, want %q", input, actual, expected)
		}
	}
}

func TestFetchScheduleUsesExplicitDates(t *testing.T) {
	p := New().(*Parser)
	p.client = newFixtureClient(t, `{"lesson_times":{"4":"16:00-17:30"},"rasp":[{"title":"Период","lessons_on_period":[{"lesson_title":"Иностранный язык","lecturers":[{"FIO":"Абызов А.А."}],"room":[],"remote":true,"lesson_time":4,"sub_group":0,"week":1,"dates":["2025-10-14","2025-10-28"],"form":"ПР. ЗАН."}]}]}`)
	p.requestInterval = 0
	p.groups["group-1485"] = sourceGroupContext{Group: sourceGroup{ID: 1485, Title: "аКШИ-11"}}
	lessons, err := p.FetchSchedule(context.Background(), "group-1485")
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) != 2 || lessons[0].Schedule.Recurrence.Kind != connector.RecurrenceDate {
		t.Fatalf("unexpected lessons: %#v", lessons)
	}
	if lessons[0].Rooms[0] != "Дистанционно" {
		t.Fatalf("remote room not retained: %#v", lessons[0].Rooms)
	}
}

func TestLiveIVGPUManagedParser(t *testing.T) {
	if os.Getenv("IVGPU_INTEGRATION_TEST") != "1" {
		t.Skip("set IVGPU_INTEGRATION_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p := New()
	groups, err := p.FetchGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 {
		t.Fatal("no groups")
	}
	lessons, err := p.FetchSchedule(ctx, groups[0].ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("groups=%d first_group=%s lessons=%d", len(groups), groups[0].Name, len(lessons))
}
