package scheduleview

import (
	"bytes"
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
