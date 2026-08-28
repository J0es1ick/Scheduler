package scheduleview

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

type exportDocument struct {
	University string         `json:"university"`
	Group      string         `json:"group"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Lessons    []exportLesson `json:"lessons"`
}

type exportLesson struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	TimeStart string `json:"time_start"`
	TimeEnd   string `json:"time_end"`
	Subject   string `json:"subject"`
	Type      string `json:"type"`
	Teacher   string `json:"teacher,omitempty"`
	Room      string `json:"room,omitempty"`
	Subgroup  int    `json:"subgroup,omitempty"`
}

func RenderJSON(request Request) ([]byte, error) {
	document := exportDocument{
		University: request.University,
		Group:      request.Group,
		From:       request.From.Format("2006-01-02"),
		To:         request.From.AddDate(0, 0, max(1, request.Days)-1).Format("2006-01-02"),
		Lessons:    flattenLessons(request),
	}
	payload, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode schedule JSON: %w", err)
	}
	return payload, nil
}

func RenderCSV(request Request) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&output)
	writer.Comma = ';'
	if err := writer.Write([]string{
		"Дата", "День недели", "Начало", "Окончание", "Предмет", "Тип", "Преподаватель", "Аудитория", "Подгруппа",
	}); err != nil {
		return nil, fmt.Errorf("write schedule CSV header: %w", err)
	}
	for _, lesson := range flattenLessons(request) {
		row := []string{
			lesson.Date,
			lesson.Weekday,
			lesson.TimeStart,
			lesson.TimeEnd,
			lesson.Subject,
			lesson.Type,
			lesson.Teacher,
			lesson.Room,
			strconv.Itoa(lesson.Subgroup),
		}
		for index := range row {
			row[index] = safeCSVCell(row[index])
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("write schedule CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("encode schedule CSV: %w", err)
	}
	return output.Bytes(), nil
}

func RenderICS(request Request) ([]byte, error) {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"PRODID:-//Scheduler//Schedule//RU",
		"X-WR-CALNAME:" + escapeICS(request.University+" · "+request.Group),
	}
	for _, lesson := range flattenLessons(request) {
		startsAt, err := parseExportDateTime(lesson.Date, lesson.TimeStart, request.From.Location())
		if err != nil {
			return nil, fmt.Errorf("encode schedule ICS start: %w", err)
		}
		endsAt, err := parseExportDateTime(lesson.Date, lesson.TimeEnd, request.From.Location())
		if err != nil {
			return nil, fmt.Errorf("encode schedule ICS end: %w", err)
		}
		digest := sha256.Sum256([]byte(strings.Join([]string{
			request.University, request.Group, lesson.Date, lesson.TimeStart, lesson.TimeEnd,
			lesson.Subject, lesson.Teacher, lesson.Room, strconv.Itoa(lesson.Subgroup),
		}, "|")))
		description := lesson.Type
		if lesson.Teacher != "" {
			description += " · " + lesson.Teacher
		}
		if lesson.Subgroup > 0 {
			description += fmt.Sprintf(" · подгруппа %d", lesson.Subgroup)
		}
		lines = append(lines,
			"BEGIN:VEVENT",
			fmt.Sprintf("UID:%x@scheduler", digest[:16]),
			"DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"),
			"DTSTART:"+startsAt.UTC().Format("20060102T150405Z"),
			"DTEND:"+endsAt.UTC().Format("20060102T150405Z"),
			"SUMMARY:"+escapeICS(lesson.Subject),
			"DESCRIPTION:"+escapeICS(description),
			"LOCATION:"+escapeICS(lesson.Room),
			"END:VEVENT",
		)
	}
	lines = append(lines, "END:VCALENDAR", "")
	return []byte(strings.Join(lines, "\r\n")), nil
}

func parseExportDateTime(date string, clock string, location *time.Location) (time.Time, error) {
	if location == nil {
		location = time.Local
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05"} {
		value, err := time.ParseInLocation(layout, date+" "+clock, location)
		if err == nil {
			return value, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date or time %q %q", date, clock)
}

func escapeICS(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ";", "\\;")
	return strings.ReplaceAll(value, ",", "\\,")
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func flattenLessons(request Request) []exportLesson {
	result := make([]exportLesson, 0)
	for _, day := range completeDays(request) {
		for _, lesson := range day.Lessons {
			result = append(result, exportLesson{
				Date:      day.Date.Format("2006-01-02"),
				Weekday:   weekdayName(day.Date),
				TimeStart: lesson.TimeStart,
				TimeEnd:   lesson.TimeEnd,
				Subject:   lesson.Subject,
				Type:      lessonTypeExportName(lesson.Type),
				Teacher:   lesson.Teacher,
				Room:      lesson.Room,
				Subgroup:  lesson.Subgroup,
			})
		}
	}
	return result
}

func weekdayName(date time.Time) string {
	names := []string{"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}
	return names[int(date.Weekday())]
}

func lessonTypeExportName(value domain.LessonType) string {
	return lessonTypeName(value)
}
