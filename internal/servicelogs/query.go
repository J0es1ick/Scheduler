package servicelogs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Cursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

type Query struct {
	Component, Module, Source, Level, Search string
	Since, Until                             time.Time
	Before                                   *Cursor
	Limit                                    int
}

func ParseQuery(values url.Values, now time.Time) (Query, error) {
	q := Query{Component: values.Get("component"), Module: values.Get("module"), Source: values.Get("source"), Level: strings.ToUpper(values.Get("level")), Search: strings.TrimSpace(values.Get("q")), Since: now.Add(-time.Hour), Until: now, Limit: 100}
	for key := range values {
		if !strings.Contains("|component|module|source|level|q|since|until|cursor|limit|", "|"+key+"|") || len(values[key]) != 1 {
			return q, errors.New("Неизвестный или повторяющийся фильтр журнала")
		}
	}
	for _, value := range []string{q.Component, q.Module, q.Source, q.Search} {
		if utf8.RuneCountInString(value) > 300 {
			return q, errors.New("Слишком длинный фильтр журнала")
		}
	}
	if q.Level != "" && q.Level != "DEBUG" && q.Level != "INFO" && q.Level != "WARN" && q.Level != "ERROR" && q.Level != "UNKNOWN" {
		return q, errors.New("Неизвестный уровень журнала")
	}
	for key, target := range map[string]*time.Time{"since": &q.Since, "until": &q.Until} {
		if value := values.Get(key); value != "" {
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return q, errors.New("Укажите время с часовым поясом")
			}
			*target = parsed
		}
	}
	if q.Since.After(q.Until) || q.Until.Sub(q.Since) > 7*24*time.Hour {
		return q, errors.New("Диапазон журнала должен составлять не более 7 дней")
	}
	if value := values.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			return q, errors.New("Размер страницы должен быть от 1 до 200")
		}
		q.Limit = limit
	}
	if value := values.Get("cursor"); value != "" {
		if len(value) > 512 {
			return q, errors.New("Некорректный курсор журнала")
		}
		data, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return q, errors.New("Некорректный курсор журнала")
		}
		var cursor Cursor
		if json.Unmarshal(data, &cursor) != nil || cursor.Time.IsZero() || len(cursor.ID) > 80 || cursor.Time.Before(q.Since) || cursor.Time.After(q.Until.Add(time.Nanosecond)) {
			return q, errors.New("Некорректный курсор журнала")
		}
		q.Before = &cursor
	}
	return q, nil
}

func encodeCursor(cursor Cursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

type Component struct {
	Name  string `json:"name"`
	State string `json:"state"`
}
type Entry struct {
	ID        string         `json:"id"`
	Time      time.Time      `json:"time"`
	Component string         `json:"component"`
	Module    string         `json:"module"`
	Source    string         `json:"source,omitempty"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields"`
}
type Page struct {
	Entries    []Entry     `json:"entries"`
	Components []Component `json:"components"`
	Modules    []string    `json:"modules"`
	NextCursor string      `json:"next_cursor,omitempty"`
	Warnings   []string    `json:"warnings"`
	CheckedAt  time.Time   `json:"checked_at"`
}
