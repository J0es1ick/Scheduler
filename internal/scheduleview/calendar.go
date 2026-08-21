package scheduleview

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

func RenderICS(request Request, timezone string) []byte {
	if strings.TrimSpace(timezone) == "" {
		timezone = "Europe/Moscow"
	}
	var output strings.Builder
	output.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Scheduler//Schedule//RU\r\nCALSCALE:GREGORIAN\r\n")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, day := range request.Schedule {
		for _, lesson := range day.Lessons {
			start, startErr := time.ParseInLocation("2006-01-02 15:04", day.Date.Format("2006-01-02")+" "+lesson.TimeStart, day.Date.Location())
			end, endErr := time.ParseInLocation("2006-01-02 15:04", day.Date.Format("2006-01-02")+" "+lesson.TimeEnd, day.Date.Location())
			if startErr != nil || endErr != nil {
				continue
			}
			fingerprint := fmt.Sprintf("%s|%s|%s|%s|%s", request.Group, day.Date.Format("2006-01-02"), lesson.TimeStart, lesson.Subject, lesson.Room)
			uid := fmt.Sprintf("%x@scheduler", sha256.Sum256([]byte(fingerprint)))
			output.WriteString("BEGIN:VEVENT\r\n")
			output.WriteString("UID:" + uid + "\r\n")
			output.WriteString("DTSTAMP:" + stamp + "\r\n")
			output.WriteString("DTSTART;TZID=" + timezone + ":" + start.Format("20060102T150405") + "\r\n")
			output.WriteString("DTEND;TZID=" + timezone + ":" + end.Format("20060102T150405") + "\r\n")
			output.WriteString("SUMMARY:" + escapeICS(lesson.Subject) + "\r\n")
			if strings.TrimSpace(lesson.Room) != "" {
				output.WriteString("LOCATION:" + escapeICS(lesson.Room) + "\r\n")
			}
			description := strings.TrimSpace(strings.Join([]string{lessonTypeName(lesson.Type), lesson.Teacher}, " · "))
			output.WriteString("DESCRIPTION:" + escapeICS(description) + "\r\n")
			output.WriteString("END:VEVENT\r\n")
		}
	}
	output.WriteString("END:VCALENDAR\r\n")
	return []byte(output.String())
}

func escapeICS(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
	)
	return replacer.Replace(value)
}
