package scheduleview

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"image"
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

func TestResearchWorkKeepsActualDurationInVisualDetails(t *testing.T) {
	lesson := domain.Lesson{Subject: "Научно-исследовательская работа", TimeStart: "08:00", TimeEnd: "17:25", Teacher: "Иванов И.И."}
	slot := visualTimeSlot(lesson, true)
	if slot != (timeSlot{"08:00", "09:35"}) {
		t.Fatalf("research block must use first row: %+v", slot)
	}
	if got := visualLessonDetails(lesson, slot); got != "08:00–17:25 · Иванов И.И." {
		t.Fatalf("duration was lost: %q", got)
	}
	if slot := visualTimeSlot(lesson, false); slot.end != "17:25" {
		t.Fatal("daily time badge changed actual duration")
	}
	if lesson.TimeEnd != "17:25" {
		t.Fatal("rendering modified source lesson")
	}
}

func TestTeacherVisualDetailsShowGroupInsteadOfRepeatingTeacher(t *testing.T) {
	lesson := domain.Lesson{Teacher: "Сизова О.В.", GroupName: "3/42", Room: "А208"}
	if got := lessonDetailsWithOptions(lesson, true); got != "группа 3/42 · А208" {
		t.Fatalf("teacher schedule details = %q", got)
	}
}

func TestRenderDaySeparatesTimeAndLessonCards(t *testing.T) {
	fontFaces, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	canvas := renderDay(Request{University: "ИГХТУ", Group: "4/147", From: date, Days: 1}, Day{
		Date: date,
		Lessons: []domain.Lesson{{
			TimeStart: "09:50", TimeEnd: "11:25", Subject: "Психология и педагогика", Type: domain.LessonTypePractice,
		}},
	}, fontFaces)

	if got := canvas.RGBAAt(70, 290); got != headerColor {
		t.Fatalf("time card color = %#v, want %#v", got, headerColor)
	}
	if got := canvas.RGBAAt(1000, 290); got != lessonTypeColor(domain.LessonTypePractice) {
		t.Fatalf("lesson card color = %#v", got)
	}
}

func TestRenderDayCentersEmptyState(t *testing.T) {
	fontFaces, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	canvas := renderDay(Request{University: "ИГХТУ", Group: "4/147", From: date, Days: 1}, Day{Date: date}, fontFaces)
	panel := image.Rect(40, 155, canvas.Bounds().Dx()-40, canvas.Bounds().Dy()-40)
	textBounds := image.Rectangle{}
	for y := panel.Min.Y; y < panel.Max.Y; y++ {
		for x := panel.Min.X; x < panel.Max.X; x++ {
			if canvas.RGBAAt(x, y) == panelColor {
				continue
			}
			point := image.Pt(x, y)
			if textBounds.Empty() {
				textBounds = image.Rect(point.X, point.Y, point.X+1, point.Y+1)
			} else {
				textBounds = textBounds.Union(image.Rect(point.X, point.Y, point.X+1, point.Y+1))
			}
		}
	}
	if textBounds.Empty() {
		t.Fatal("empty-state text was not rendered")
	}
	textCenter := textBounds.Min.Add(textBounds.Size().Div(2))
	panelCenter := panel.Min.Add(panel.Size().Div(2))
	if delta := textCenter.X - panelCenter.X; delta < -3 || delta > 3 {
		t.Fatalf("empty-state horizontal center = %d, want %d", textCenter.X, panelCenter.X)
	}
	if delta := textCenter.Y - panelCenter.Y; delta < -8 || delta > 8 {
		t.Fatalf("empty-state vertical center = %d, want %d", textCenter.Y, panelCenter.Y)
	}
}

func TestRenderDayUsesEqualVerticalPanelPadding(t *testing.T) {
	fontFaces, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, time.September, 8, 0, 0, 0, 0, time.Local)
	lessons := []domain.Lesson{
		{TimeStart: "08:00", TimeEnd: "09:35", Subject: "Первая пара", Type: domain.LessonTypePractice},
		{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Вторая пара", Type: domain.LessonTypeLecture},
		{TimeStart: "12:10", TimeEnd: "13:45", Subject: "Третья пара", Type: domain.LessonTypeLab},
	}
	canvas := renderDay(Request{University: "ИГХТУ", Group: "4/147", From: date, Days: 1}, Day{Date: date, Lessons: lessons}, fontFaces)
	panel := image.Rect(35, 150, canvas.Bounds().Dx()-35, canvas.Bounds().Dy()-35)
	const lessonHeight = 145
	lastRowBottom := panel.Min.Y + 20 + (len(lessons)-1)*lessonHeight + lessonHeight - 12
	if top, bottom := 20, panel.Max.Y-lastRowBottom; top != bottom {
		t.Fatalf("daily panel padding top=%d bottom=%d", top, bottom)
	}
	if got, want := canvas.Bounds().Dy(), 213+len(lessons)*lessonHeight; got != want {
		t.Fatalf("daily image height=%d, want %d", got, want)
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
			Type: domain.LessonTypeLecture, Room: "А-101", GroupName: "4/147",
		}}}},
	}
	jsonPayload, err := RenderJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"university": "ИГХТУ"`, `"subject": "Математика, часть 1"`, `"date": "2026-09-07"`, `"group": "4/147"`} {
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
	icsPayload, err := RenderICS(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"BEGIN:VCALENDAR", "BEGIN:VEVENT", "DTSTART:20260907T050000Z", "DTEND:20260907T063500Z", "SUMMARY:Математика\\, часть 1", "LOCATION:А-101", "END:VCALENDAR"} {
		if !strings.Contains(string(icsPayload), expected) {
			t.Fatalf("ICS does not contain %q:\n%s", expected, icsPayload)
		}
	}
}

func TestCalendarExportEscapesAllLineBreaks(t *testing.T) {
	value := escapeICS("Предмет\rBEGIN:VEVENT\nLOCATION:Другая\r\nАудитория")
	if strings.ContainsAny(value, "\r\n") || value != "Предмет\\nBEGIN:VEVENT\\nLOCATION:Другая\\nАудитория" {
		t.Fatalf("calendar field can inject properties: %q", value)
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
	if len(records) != 2 || len(records[1]) != 10 {
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
	if got, want := visualTimeSlot(lesson, true), (timeSlot{start: "08:00", end: "09:35"}); got != want {
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
	slots := weekSlots(days, true)
	if len(slots) != 1 || slots[0] != (timeSlot{start: "08:00", end: "09:35"}) {
		t.Fatalf("unexpected visual slots: %#v", slots)
	}
}

func TestRenderPNGRejectsPathologicalScheduleBeforeAllocation(t *testing.T) {
	date := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	lessons := make([]domain.Lesson, maxLessonsPerRenderDay+1)
	for index := range lessons {
		lessons[index] = domain.Lesson{TimeStart: "08:00", TimeEnd: "09:35", Subject: "Занятие"}
	}
	_, err := RenderPNG(Request{From: date, Days: 1, Schedule: []Day{{Date: date, Lessons: lessons}}})
	if !errors.Is(err, ErrRenderLimit) {
		t.Fatalf("RenderPNG() error = %v, want ErrRenderLimit", err)
	}
}

func TestRenderPNGContextStopsWaitingAfterCancellation(t *testing.T) {
	for range maxConcurrentPNGRenders {
		pngRenderSlots <- struct{}{}
	}
	defer func() {
		for len(pngRenderSlots) > 0 {
			<-pngRenderSlots
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RenderPNGContext(ctx, Request{From: time.Now(), Days: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RenderPNGContext() error = %v, want context.Canceled", err)
	}
}
