package scheduleview

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type Day struct {
	Date    time.Time
	Lessons []domain.Lesson
}

type Request struct {
	University string
	Group      string
	From       time.Time
	Days       int
	Schedule   []Day
}

var (
	canvasColor = hex("f5f1e8")
	panelColor  = hex("fffdf8")
	lineColor   = hex("d8d1c4")
	textColor   = hex("292722")
	mutedColor  = hex("756f65")
	headerColor = hex("eadba8")
)

type faces struct {
	title  font.Face
	header font.Face
	body   font.Face
	small  font.Face
}

func RenderPNG(request Request) ([]byte, error) {
	if request.Days <= 0 {
		request.Days = 1
	}
	fontFaces, err := loadFaces()
	if err != nil {
		return nil, err
	}
	days := completeDays(request)
	var imageValue *image.RGBA
	if request.Days == 1 {
		imageValue = renderDay(request, days[0], fontFaces)
	} else {
		imageValue = renderWeeks(request, days, fontFaces)
	}
	var output bytes.Buffer
	if err = png.Encode(&output, imageValue); err != nil {
		return nil, fmt.Errorf("encode schedule PNG: %w", err)
	}
	return output.Bytes(), nil
}

func loadFaces() (*faces, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse regular font: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse bold font: %w", err)
	}
	face := func(value *sfnt.Font, size float64) (font.Face, error) {
		return opentype.NewFace(value, &opentype.FaceOptions{Size: size, DPI: 96, Hinting: font.HintingFull})
	}
	title, err := face(bold, 25)
	if err != nil {
		return nil, err
	}
	header, err := face(bold, 16)
	if err != nil {
		return nil, err
	}
	body, err := face(regular, 14)
	if err != nil {
		return nil, err
	}
	small, err := face(regular, 11)
	if err != nil {
		return nil, err
	}
	return &faces{title: title, header: header, body: body, small: small}, nil
}

func completeDays(request Request) []Day {
	byDate := make(map[string][]domain.Lesson, len(request.Schedule))
	for _, day := range request.Schedule {
		byDate[day.Date.Format("2006-01-02")] = day.Lessons
	}
	days := make([]Day, request.Days)
	for index := range days {
		date := request.From.AddDate(0, 0, index)
		days[index] = Day{Date: date, Lessons: byDate[date.Format("2006-01-02")]}
	}
	return days
}

func renderDay(request Request, day Day, faces *faces) *image.RGBA {
	const width = 1100
	lessonHeight := 145
	height := 220 + max(1, len(day.Lessons))*lessonHeight + 60
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{canvasColor}, image.Point{}, draw.Src)
	drawTitle(canvas, request, day.Date, day.Date, faces)
	panel := image.Rect(35, 150, width-35, height-35)
	fillRect(canvas, panel, panelColor)
	strokeRect(canvas, panel, lineColor, 2)
	if len(day.Lessons) == 0 {
		drawText(canvas, faces.header, textColor, panel.Min.X+35, panel.Min.Y+70, "Занятий нет")
		return canvas
	}
	y := panel.Min.Y + 20
	for _, lesson := range day.Lessons {
		lessonRect := image.Rect(panel.Min.X+18, y, panel.Max.X-18, y+lessonHeight-12)
		fillRect(canvas, lessonRect, lessonTypeColor(lesson.Type))
		strokeRect(canvas, lessonRect, lineColor, 1)
		clipped := clippedCanvas(canvas, lessonRect.Inset(2))
		slot := visualTimeSlot(lesson)
		drawText(clipped, faces.header, textColor, lessonRect.Min.X+18, lessonRect.Min.Y+29,
			slot.start+"–"+slot.end+"  ·  "+strings.ToUpper(lessonTypeName(lesson.Type)))
		lineY := drawWrapped(clipped, faces.header, textColor, lessonRect.Min.X+18, lessonRect.Min.Y+58,
			lessonRect.Dx()-36, 22, visualSubject(lesson.Subject), 2)
		details := lessonDetails(lesson)
		if details != "" {
			drawWrapped(clipped, faces.body, mutedColor, lessonRect.Min.X+18, lineY+7, lessonRect.Dx()-36, 20, details, 2)
		}
		y += lessonHeight
	}
	return canvas
}

func renderWeeks(request Request, days []Day, faces *faces) *image.RGBA {
	const (
		width        = 1900
		titleHeight  = 150
		weekGap      = 34
		bottomMargin = 45
	)
	weekCount := (len(days) + 6) / 7
	heights := make([]int, weekCount)
	for index := 0; index < weekCount; index++ {
		from := index * 7
		to := min(from+7, len(days))
		heights[index] = weekHeight(days[from:to])
	}
	height := titleHeight + bottomMargin + (weekCount-1)*weekGap
	for _, value := range heights {
		height += value
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{canvasColor}, image.Point{}, draw.Src)
	drawTitle(canvas, request, request.From, request.From.AddDate(0, 0, request.Days-1), faces)
	y := titleHeight
	for index := 0; index < weekCount; index++ {
		from := index * 7
		to := min(from+7, len(days))
		renderWeek(canvas, image.Pt(30, y), width-60, days[from:to], faces)
		y += heights[index] + weekGap
	}
	return canvas
}

func weekHeight(days []Day) int {
	slots := weekSlots(days)
	if len(slots) == 0 {
		return 72 + 140
	}
	height := 72
	for _, slot := range slots {
		height += slotHeight(days, slot)
	}
	return height
}

func renderWeek(canvas *image.RGBA, origin image.Point, width int, days []Day, faces *faces) {
	const timeWidth = 145
	height := weekHeight(days)
	panel := image.Rect(origin.X, origin.Y, origin.X+width, origin.Y+height)
	fillRect(canvas, panel, panelColor)
	strokeRect(canvas, panel, lineColor, 2)
	columnWidth := (width - timeWidth) / len(days)
	fillRect(canvas, image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, panel.Min.Y+72), headerColor)
	drawText(canvas, faces.header, textColor, panel.Min.X+20, panel.Min.Y+43, "Время")
	for index, day := range days {
		x := panel.Min.X + timeWidth + index*columnWidth
		strokeVertical(canvas, x, panel.Min.Y, panel.Max.Y, lineColor)
		label := weekdayShort(day.Date) + "  " + day.Date.Format("02.01")
		drawCentered(canvas, faces.header, textColor, x, x+columnWidth, panel.Min.Y+43, label)
	}
	slots := weekSlots(days)
	y := panel.Min.Y + 72
	for _, slot := range slots {
		rowHeight := slotHeight(days, slot)
		strokeHorizontal(canvas, panel.Min.X, panel.Max.X, y, lineColor)
		drawCentered(canvas, faces.header, textColor, panel.Min.X, panel.Min.X+timeWidth, y+46, slot.start)
		drawCentered(canvas, faces.small, mutedColor, panel.Min.X, panel.Min.X+timeWidth, y+72, slot.end)
		for dayIndex, day := range days {
			cell := image.Rect(
				panel.Min.X+timeWidth+dayIndex*columnWidth+1,
				y+1,
				panel.Min.X+timeWidth+(dayIndex+1)*columnWidth,
				y+rowHeight,
			)
			lessons := lessonsAt(day.Lessons, slot)
			if len(lessons) == 0 {
				continue
			}
			subHeight := cell.Dy() / len(lessons)
			for lessonIndex, lesson := range lessons {
				sub := image.Rect(cell.Min.X, cell.Min.Y+lessonIndex*subHeight, cell.Max.X, cell.Min.Y+(lessonIndex+1)*subHeight)
				fillRect(canvas, sub, lessonTypeColor(lesson.Type))
				if lessonIndex > 0 {
					strokeHorizontal(canvas, sub.Min.X, sub.Max.X, sub.Min.Y, lineColor)
				}
				clipped := clippedCanvas(canvas, sub.Inset(2))
				lineY := drawWrapped(clipped, faces.header, textColor, sub.Min.X+9, sub.Min.Y+22, sub.Dx()-18, 18, visualSubject(lesson.Subject), 3)
				details := lessonDetails(lesson)
				if details != "" && lineY < sub.Max.Y-15 {
					drawWrapped(clipped, faces.small, mutedColor, sub.Min.X+9, lineY+5, sub.Dx()-18, 15, details, 3)
				}
			}
		}
		y += rowHeight
	}
	emptyStateY := panel.Min.Y + 72 + min(70, max(45, (panel.Dy()-72)/3))
	for dayIndex, day := range days {
		if len(day.Lessons) != 0 {
			continue
		}
		minX := panel.Min.X + timeWidth + dayIndex*columnWidth
		drawCentered(canvas, faces.small, mutedColor, minX, minX+columnWidth, emptyStateY, "Занятий нет")
	}
}

type timeSlot struct{ start, end string }

func weekSlots(days []Day) []timeSlot {
	unique := map[timeSlot]struct{}{}
	for _, day := range days {
		for _, lesson := range day.Lessons {
			unique[visualTimeSlot(lesson)] = struct{}{}
		}
	}
	result := make([]timeSlot, 0, len(unique))
	for slot := range unique {
		result = append(result, slot)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].start == result[j].start {
			return result[i].end < result[j].end
		}
		return result[i].start < result[j].start
	})
	return result
}

func lessonsAt(lessons []domain.Lesson, slot timeSlot) []domain.Lesson {
	result := make([]domain.Lesson, 0, 2)
	for _, lesson := range lessons {
		if visualTimeSlot(lesson) == slot {
			result = append(result, lesson)
		}
	}
	return result
}

func visualTimeSlot(lesson domain.Lesson) timeSlot {
	if lesson.TimeStart == "08:00" && isResearchWork(lesson.Subject) {
		return timeSlot{start: "08:00", end: "09:35"}
	}
	return timeSlot{start: lesson.TimeStart, end: lesson.TimeEnd}
}

func isResearchWork(subject string) bool {
	return strings.Contains(strings.ToLower(subject), "научно-исследовательск")
}

func visualSubject(subject string) string {
	value := strings.TrimSpace(subject)
	value = strings.TrimSpace(strings.TrimSuffix(value, "- - -"))
	return value
}

func slotHeight(days []Day, slot timeSlot) int {
	maximum := 1
	for _, day := range days {
		maximum = max(maximum, len(lessonsAt(day.Lessons, slot)))
	}
	return maximum * 140
}

func drawTitle(canvas *image.RGBA, request Request, from, to time.Time, faces *faces) {
	title := strings.TrimSpace(request.University + " · " + request.Group)
	title = strings.Trim(title, " ·")
	drawText(canvas, faces.title, textColor, 38, 53, title)
	period := from.Format("02.01.2006")
	if !sameDate(from, to) {
		period += " — " + to.Format("02.01.2006")
	}
	drawText(canvas, faces.body, mutedColor, 38, 91, period)
	drawLegend(canvas, faces, 38, 124)
}

func drawLegend(canvas *image.RGBA, faces *faces, x, y int) {
	items := []struct {
		label string
		type_ domain.LessonType
	}{
		{"Лекция", domain.LessonTypeLecture},
		{"Практика", domain.LessonTypePractice},
		{"Лабораторная", domain.LessonTypeLab},
		{"Семинар", domain.LessonTypeSeminar},
		{"Другое", domain.LessonTypeOther},
	}
	for _, item := range items {
		fillRect(canvas, image.Rect(x, y-18, x+18, y), lessonTypeColor(item.type_))
		strokeRect(canvas, image.Rect(x, y-18, x+18, y), lineColor, 1)
		drawText(canvas, faces.small, mutedColor, x+26, y-3, item.label)
		x += 42 + textWidth(faces.small, item.label)
	}
}

func lessonDetails(lesson domain.Lesson) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(lesson.Teacher) != "" {
		parts = append(parts, lesson.Teacher)
	}
	if strings.TrimSpace(lesson.Room) != "" {
		parts = append(parts, lesson.Room)
	}
	if lesson.Subgroup > 0 {
		parts = append(parts, fmt.Sprintf("подгруппа %d", lesson.Subgroup))
	}
	return strings.Join(parts, " · ")
}

func lessonTypeName(value domain.LessonType) string {
	switch value {
	case domain.LessonTypeLecture:
		return "лекция"
	case domain.LessonTypePractice:
		return "практика"
	case domain.LessonTypeLab:
		return "лабораторная"
	case domain.LessonTypeSeminar:
		return "семинар"
	case domain.LessonTypeExam:
		return "экзамен"
	case domain.LessonTypeCredit:
		return "зачёт"
	case domain.LessonTypeConsultation:
		return "консультация"
	default:
		return "занятие"
	}
}

func lessonTypeColor(value domain.LessonType) color.RGBA {
	switch value {
	case domain.LessonTypeLecture:
		return hex("eef7df")
	case domain.LessonTypePractice:
		return hex("faeaf2")
	case domain.LessonTypeLab:
		return hex("def4f5")
	case domain.LessonTypeSeminar:
		return hex("fff7d8")
	case domain.LessonTypeExam, domain.LessonTypeCredit:
		return hex("f9dfda")
	case domain.LessonTypeConsultation:
		return hex("eee8f8")
	default:
		return hex("eceae4")
	}
}

func drawWrapped(canvas *image.RGBA, face font.Face, ink color.Color, x, y, width, lineHeight int, value string, maxLines int) int {
	lines := wrap(face, value, width)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := strings.TrimSpace(lines[len(lines)-1])
		for textWidth(face, last+"…") > width && len([]rune(last)) > 1 {
			runes := []rune(last)
			last = string(runes[:len(runes)-1])
		}
		lines[len(lines)-1] = strings.TrimSpace(last) + "…"
	}
	for _, line := range lines {
		drawText(canvas, face, ink, x, y, line)
		y += lineHeight
	}
	return y
}

func wrap(face font.Face, value string, width int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, len(words))
	current := ""
	for _, word := range words {
		chunks := breakWord(face, word, width)
		for chunkIndex, chunk := range chunks {
			if current == "" {
				current = chunk
			} else if chunkIndex == 0 && textWidth(face, current+" "+chunk) <= width {
				current += " " + chunk
			} else {
				lines = append(lines, current)
				current = chunk
			}
			if chunkIndex < len(chunks)-1 {
				lines = append(lines, current)
				current = ""
			}
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func breakWord(face font.Face, word string, width int) []string {
	if textWidth(face, word) <= width {
		return []string{word}
	}
	result := make([]string, 0, 2)
	current := make([]rune, 0, len([]rune(word)))
	for _, character := range []rune(word) {
		candidate := string(append(current, character))
		if len(current) > 0 && textWidth(face, candidate) > width {
			result = append(result, string(current))
			current = current[:0]
		}
		current = append(current, character)
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}

func drawText(canvas *image.RGBA, face font.Face, ink color.Color, x, baseline int, value string) {
	drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(value)
}

func clippedCanvas(canvas *image.RGBA, bounds image.Rectangle) *image.RGBA {
	bounds = bounds.Intersect(canvas.Bounds())
	return canvas.SubImage(bounds).(*image.RGBA)
}

func drawCentered(canvas *image.RGBA, face font.Face, ink color.Color, minX, maxX, baseline int, value string) {
	drawText(canvas, face, ink, minX+(maxX-minX-textWidth(face, value))/2, baseline, value)
}

func textWidth(face font.Face, value string) int {
	return font.MeasureString(face, value).Ceil()
}

func fillRect(canvas *image.RGBA, rectangle image.Rectangle, fill color.Color) {
	draw.Draw(canvas, rectangle, &image.Uniform{fill}, image.Point{}, draw.Src)
}

func strokeRect(canvas *image.RGBA, rectangle image.Rectangle, ink color.Color, thickness int) {
	fillRect(canvas, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Min.Y+thickness), ink)
	fillRect(canvas, image.Rect(rectangle.Min.X, rectangle.Max.Y-thickness, rectangle.Max.X, rectangle.Max.Y), ink)
	fillRect(canvas, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Min.X+thickness, rectangle.Max.Y), ink)
	fillRect(canvas, image.Rect(rectangle.Max.X-thickness, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y), ink)
}

func strokeVertical(canvas *image.RGBA, x, minY, maxY int, ink color.Color) {
	fillRect(canvas, image.Rect(x, minY, x+1, maxY), ink)
}

func strokeHorizontal(canvas *image.RGBA, minX, maxX, y int, ink color.Color) {
	fillRect(canvas, image.Rect(minX, y, maxX, y+1), ink)
}

func weekdayShort(date time.Time) string {
	names := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}
	return names[int(date.Weekday())]
}

func sameDate(left, right time.Time) bool {
	ly, lm, ld := left.Date()
	ry, rm, rd := right.Date()
	return ly == ry && lm == rm && ld == rd
}

func hex(value string) color.RGBA {
	var r, g, b uint8
	_, _ = fmt.Sscanf(value, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
