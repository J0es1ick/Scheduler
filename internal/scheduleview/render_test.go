package scheduleview

import (
	"bytes"
	"encoding/csv"
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

func TestRenderPNGWaitsWhenConcurrencyLimitIsFull(t *testing.T) {
	for range maxConcurrentPNGRenders {
		pngRenderSlots <- struct{}{}
	}
	defer func() {
		for len(pngRenderSlots) > 0 {
			<-pngRenderSlots
		}
	}()

	completed := make(chan error, 1)
	go func() {
		_, err := RenderPNG(Request{From: time.Now(), Days: 1})
		completed <- err
	}()

	select {
	case err := <-completed:
		t.Fatalf("render bypassed concurrency limit: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	<-pngRenderSlots
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("render did not resume after a slot became available")
	}
}

func TestScheduleExportsIncludeLesson(t *testing.T) {
	date := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	request := Request{
		University: "ИГХТУ",
		Group:      "4/147",
		From:       date,
		Days:       1,
		Schedule: []Day{{Date: date, Lessons: []domain.Lesson{{
			TimeStart: "08:00", TimeEnd: "09:35", Subject: "Математика, часть 1",
			Type: domain.LessonTypeLecture, Room: "А-101",
		}}}},
	}
	jsonPayload, err := RenderJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"university": "ИГХТУ"`, `"subject": "Математика, часть 1"`, `"date": "2026-09-07"`} {
		if !strings.Contains(string(jsonPayload), expected) {
			t.Fatalf("JSON does not contain %q:\n%s", expected, jsonPayload)
		}
	}
	csvPayload, err := RenderCSV(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(csvPayload, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV has no UTF-8 BOM")
	}
	for _, expected := range []string{"2026-09-07", "Математика, часть 1", "А-101"} {
		if !strings.Contains(string(csvPayload), expected) {
			t.Fatalf("CSV does not contain %q:\n%s", expected, csvPayload)
		}
	}
}

func TestSafeCSVCellPreventsFormulaInjection(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "hyperlink", value: `=HYPERLINK("https://example.test")`, want: `'=HYPERLINK("https://example.test")`},
		{name: "command", value: "+cmd|' /C calc'!A0", want: "'+cmd|' /C calc'!A0"},
		{name: "minus sum", value: "-SUM(A1:A2)", want: "'-SUM(A1:A2)"},
		{name: "leading whitespace and sum", value: " \t\r=SUM(A1:A2)", want: "' \t\r=SUM(A1:A2)"},
		{name: "at marker", value: "\t@SUM(A1:A2)", want: "'\t@SUM(A1:A2)"},
		{name: "plain function name", value: "SUM(A1:A2)", want: "SUM(A1:A2)"},
		{name: "ordinary text", value: "Математика", want: "Математика"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := safeCSVCell(test.value); got != test.want {
				t.Fatalf("safeCSVCell(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestRenderCSVEscapesUntrustedLessonFields(t *testing.T) {
	date := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	payload, err := RenderCSV(Request{
		From: date,
		Days: 1,
		Schedule: []Day{{Date: date, Lessons: []domain.Lesson{{
			TimeStart: "08:00",
			TimeEnd:   "09:35",
			Subject:   `=HYPERLINK("https://example.test")`,
			Teacher:   "+cmd",
			Room:      " \t\r=SUM(A1:A2)",
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(bytes.NewReader(payload[3:]))
	reader.Comma = ';'
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse rendered CSV: %v", err)
	}
	if len(records) != 2 || len(records[1]) != 9 {
		t.Fatalf("unexpected CSV records: %#v", records)
	}
	for index, want := range map[int]string{
		4: `'=HYPERLINK("https://example.test")`,
		6: "'+cmd",
		7: "' \t\r=SUM(A1:A2)",
	} {
		if got := records[1][index]; got != want {
			t.Fatalf("CSV cell %d = %q, want %q", index, got, want)
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
