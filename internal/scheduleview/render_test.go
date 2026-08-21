package scheduleview

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestRenderPNGProducesReadableImage(t *testing.T) {
	date := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	payload, err := RenderPNG(Request{
		University: "ИГХТУ",
		Group:      "4/147",
		From:       date,
		Days:       7,
		Schedule: []Day{{Date: date, Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Научно-исследовательская работа",
			Type: domain.LessonTypeOther, Teacher: "Иванов И.И.", Room: "А-101",
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	imageValue, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	if imageValue.Bounds().Dx() < 1000 || imageValue.Bounds().Dy() < 300 {
		t.Fatalf("unexpected image size: %v", imageValue.Bounds())
	}
}

func TestRenderICSIncludesLesson(t *testing.T) {
	date := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	payload := RenderICS(Request{
		Group: "4/147",
		Schedule: []Day{{Date: date, Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика, часть 1",
			Type: domain.LessonTypeLecture, Room: "А-101",
		}}}},
	}, "Europe/Moscow")
	text := string(payload)
	for _, expected := range []string{"BEGIN:VCALENDAR", "SUMMARY:Математика\\, часть 1", "DTSTART;TZID=Europe/Moscow:20260907T080000"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("calendar does not contain %q:\n%s", expected, text)
		}
	}
}

func TestResearchWorkUsesFirstStandardSlotInVisualTable(t *testing.T) {
	lesson := domain.Lesson{
		TimeStart: "08:00",
		TimeEnd:   "17:25",
		Subject:   "Научно-исследовательская работа - - -",
		Type:      domain.LessonTypeOther,
	}
	if got, want := visualTimeSlot(lesson), (timeSlot{start: "08:00", end: "09:35"}); got != want {
		t.Fatalf("visualTimeSlot() = %#v, want %#v", got, want)
	}
	if got, want := visualSubject(lesson.Subject), "Научно-исследовательская работа"; got != want {
		t.Fatalf("visualSubject() = %q, want %q", got, want)
	}
}

func TestWrapBreaksWordWiderThanCell(t *testing.T) {
	fontFaces, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	const width = 90
	lines := wrap(fontFaces.header, "Сверхдлинноеназваниебезпробелов", width)
	if len(lines) < 2 {
		t.Fatalf("long word was not wrapped: %#v", lines)
	}
	for _, line := range lines {
		if measured := textWidth(fontFaces.header, line); measured > width {
			t.Fatalf("wrapped line %q is %d px wide, limit is %d", line, measured, width)
		}
	}
}

func TestWeekSlotsDoNotCreateAllDayResearchRow(t *testing.T) {
	days := []Day{{Lessons: []domain.Lesson{
		{TimeStart: "08:00", TimeEnd: "17:25", Subject: "Научно-исследовательская работа"},
		{TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика"},
	}}}
	slots := weekSlots(days)
	if len(slots) != 1 || slots[0] != (timeSlot{start: "08:00", end: "09:35"}) {
		t.Fatalf("unexpected visual slots: %#v", slots)
	}
}
