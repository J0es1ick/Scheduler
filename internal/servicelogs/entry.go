package servicelogs

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var textLevelPattern = regexp.MustCompile(`(?i)\b(?:level|severity)=["']?([a-z]+)`)

func newEntry(component string, at time.Time, message string) Entry {
	entry := Entry{ID: uuid.NewString(), Time: at.UTC(), Component: component, Module: component, Level: "UNKNOWN", Message: RedactText(message), Fields: map[string]any{}}
	var fields map[string]any
	if json.Unmarshal([]byte(message), &fields) == nil && fields != nil {
		fields = redactValue(fields).(map[string]any)
		entry.Fields = fields
		if text := fieldString(fields, "msg", "message"); text != "" {
			entry.Message = text
		}
		entry.Level = normalizeLevel(fieldString(fields, "level", "severity"))
		entry.Source = fieldString(fields, "source_id", "data_source_id", "dataSourceID", "source")
		if module := fieldString(fields, "module", "worker", "component"); module != "" {
			entry.Module = module
		}
		delete(fields, "time")
		delete(fields, "ts")
		delete(fields, "level")
		delete(fields, "msg")
		delete(fields, "message")
	} else {
		upper := strings.ToUpper(message)
		for _, level := range []string{"PANIC", "FATAL", "ERROR", "WARNING", "WARN", "INFO", "DEBUG", "LOG"} {
			if strings.Contains(upper, level+":") || strings.HasPrefix(upper, level+" ") {
				entry.Level = normalizeLevel(level)
				break
			}
		}
		if match := textLevelPattern.FindStringSubmatch(message); match != nil {
			entry.Level = normalizeLevel(match[1])
		}
	}
	if entry.Module == component {
		lower := strings.ToLower(entry.Message)
		for _, module := range []string{"parser", "notification", "reminder", "privacy", "connector"} {
			if strings.HasPrefix(lower, module+":") || strings.HasPrefix(lower, module+" ") || strings.HasPrefix(lower, "lesson "+module+" ") {
				entry.Module = module
				break
			}
		}
		if strings.HasPrefix(lower, "bot outbox ") {
			entry.Module = "notification"
		}
	}
	if len(entry.Message) > 8192 {
		entry.Message = string([]rune(entry.Message)[:min(8192, len([]rune(entry.Message)))]) + "… [обрезано]"
	}
	if data, _ := json.Marshal(entry.Fields); len(data) > 32768 {
		entry.Fields = map[string]any{"truncated": true, "note": "Поля записи превышают 32 КБ"}
	}
	return entry
}

func normalizeLevel(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR", "FATAL", "PANIC":
		return "ERROR"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO", "LOG", "NOTICE":
		return "INFO"
	case "DEBUG", "TRACE":
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}
