package ivgpu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
)

const (
	ParserID           = "ivgpu"
	UniversityID       = "ivgpu"
	defaultAPIURL      = "https://raspisanie.ivgpu.ru"
	defaultScheduleURL = "https://ivgpu.ru/raspisanie"
	maxResponseBytes   = 16 << 20
)

var timeRangePattern = regexp.MustCompile(`(?i)(\d{1,2}:\d{2})\s*[-–—]\s*(\d{1,2}:\d{2})`)

func Manifest() managed.Manifest {
	return managed.NormalizeManifest(managed.Manifest{
		ContractVersion: managed.ContractVersion,
		ParserID:        ParserID,
		Version:         "1.0.0",
		DisplayName:     "ИВГПУ · управляемый парсер",
		Description:     "Официальный JSON API расписания ИВГПУ. Запуск, повторы и публикацию выполняет Scheduler.",
		Institution: connector.Institution{
			ExternalID:  UniversityID,
			Name:        "ИВГПУ",
			FullName:    "Ивановский государственный политехнический университет",
			ScheduleURL: defaultScheduleURL,
			Timezone:    "Europe/Moscow",
			Locale:      "ru-RU",
		},
		MaintainerName: "Scheduler contributors",
		MaintainerURL:  "https://github.com/J0es1ick/Scheduler/tree/master/integrations/ivgpu",
		UpdateInterval: time.Hour,
	})
}

func New() managed.Parser {
	return &Parser{
		baseURL:         defaultAPIURL,
		client:          &http.Client{Timeout: 25 * time.Second},
		requestInterval: 150 * time.Millisecond,
		groups:          make(map[string]sourceGroupContext),
	}
}

type Parser struct {
	baseURL         string
	client          *http.Client
	requestInterval time.Duration

	mu      sync.RWMutex
	groups  map[string]sourceGroupContext
	paceMu  sync.Mutex
	nextHit time.Time
}

type sourceInstitute struct {
	Abbreviation string        `json:"abr"`
	Title        string        `json:"title"`
	Groups       []sourceGroup `json:"groups"`
}

type sourceGroup struct {
	ID      int        `json:"id"`
	Title   string     `json:"title"`
	EduForm sourceText `json:"eduForm"`
}

type sourceText string

func (value *sourceText) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*value = sourceText(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*value = sourceText(number.String())
		return nil
	}
	if string(data) == "null" {
		*value = ""
		return nil
	}
	return fmt.Errorf("unsupported text value %s", string(data))
}

type sourceGroupContext struct {
	Group     sourceGroup
	Institute sourceInstitute
}

type sourceSchedule struct {
	LessonTimes map[string]string `json:"lesson_times"`
	Periods     []sourcePeriod    `json:"rasp"`
}

type sourcePeriod struct {
	Title    string         `json:"title"`
	Modified string         `json:"last_modify"`
	Lessons  []sourceLesson `json:"lessons_on_period"`
}

type sourceLesson struct {
	Title      string           `json:"lesson_title"`
	Lecturers  []sourceLecturer `json:"lecturers"`
	Rooms      []string         `json:"room"`
	Remote     bool             `json:"remote"`
	LessonTime int              `json:"lesson_time"`
	Subgroup   int              `json:"sub_group"`
	Week       int              `json:"week"`
	Dates      []string         `json:"dates"`
	Form       string           `json:"form"`
}

type sourceLecturer struct {
	Name string `json:"FIO"`
}

func (p *Parser) Manifest() managed.Manifest { return Manifest() }

func (p *Parser) FetchGroups(ctx context.Context) ([]managed.Group, error) {
	var institutes []sourceInstitute
	if err := p.getJSON(ctx, "/api/grouplist/", &institutes); err != nil {
		return nil, fmt.Errorf("load IVGPU group list: %w", err)
	}
	nameCounts := make(map[string]int)
	for _, institute := range institutes {
		for _, group := range institute.Groups {
			nameCounts[strings.ToLower(strings.TrimSpace(group.Title))]++
		}
	}
	items := make([]managed.Group, 0)
	lookup := make(map[string]sourceGroupContext)
	for _, institute := range institutes {
		for _, group := range institute.Groups {
			if group.ID <= 0 || strings.TrimSpace(group.Title) == "" {
				return nil, fmt.Errorf("IVGPU group list contains invalid group id=%d title=%q", group.ID, group.Title)
			}
			externalID := "group-" + strconv.Itoa(group.ID)
			if _, duplicate := lookup[externalID]; duplicate {
				return nil, fmt.Errorf("IVGPU group id %d is duplicated", group.ID)
			}
			name := strings.TrimSpace(group.Title)
			if nameCounts[strings.ToLower(name)] > 1 && strings.TrimSpace(institute.Abbreviation) != "" {
				name += " (" + strings.TrimSpace(institute.Abbreviation) + ")"
			}
			lookup[externalID] = sourceGroupContext{Group: group, Institute: institute}
			items = append(items, managed.Group{
				ExternalID: externalID,
				Name:       name,
				Metadata: map[string]string{
					"source_group_id": strconv.Itoa(group.ID),
					"institute":       strings.TrimSpace(institute.Title),
					"institute_code":  strings.TrimSpace(institute.Abbreviation),
					"education_form":  strings.TrimSpace(string(group.EduForm)),
				},
			})
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("IVGPU returned an empty group list")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ExternalID < items[j].ExternalID })
	p.mu.Lock()
	p.groups = lookup
	p.mu.Unlock()
	return items, nil
}

func (p *Parser) FetchSchedule(ctx context.Context, externalGroupID string) ([]managed.Lesson, error) {
	p.mu.RLock()
	item, ok := p.groups[externalGroupID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown IVGPU group %q", externalGroupID)
	}
	var schedule sourceSchedule
	path := "/api/rasp/?group_id=" + url.QueryEscape(strconv.Itoa(item.Group.ID))
	if err := p.getJSON(ctx, path, &schedule); err != nil {
		return nil, fmt.Errorf("load IVGPU schedule for group %d: %w", item.Group.ID, err)
	}
	lessons := make([]managed.Lesson, 0)
	seen := make(map[string]bool)
	for _, period := range schedule.Periods {
		for _, sourceLesson := range period.Lessons {
			start, end, err := lessonTime(schedule.LessonTimes, sourceLesson.LessonTime)
			if err != nil {
				return nil, fmt.Errorf("lesson %q: %w", sourceLesson.Title, err)
			}
			teachers := make([]string, 0, len(sourceLesson.Lecturers))
			for _, lecturer := range sourceLesson.Lecturers {
				if name := strings.TrimSpace(lecturer.Name); name != "" {
					teachers = append(teachers, name)
				}
			}
			rooms := cleanStrings(sourceLesson.Rooms)
			if sourceLesson.Remote && len(rooms) == 0 {
				rooms = []string{"Дистанционно"}
			}
			for _, rawDate := range sourceLesson.Dates {
				date, err := parseDate(rawDate)
				if err != nil {
					return nil, err
				}
				fingerprint := strings.Join([]string{
					strconv.Itoa(item.Group.ID), date.Format(time.DateOnly), start, end,
					strings.TrimSpace(sourceLesson.Title), strings.TrimSpace(sourceLesson.Form),
					strings.Join(teachers, ","), strings.Join(rooms, ","), strconv.Itoa(sourceLesson.Subgroup),
				}, "\x00")
				if seen[fingerprint] {
					continue
				}
				seen[fingerprint] = true
				lessons = append(lessons, managed.Lesson{
					ExternalID: "lesson-" + shortHash(fingerprint),
					Subject:    strings.TrimSpace(sourceLesson.Title),
					Type:       lessonType(sourceLesson.Form),
					Teachers:   teachers,
					Rooms:      rooms,
					Subgroup:   max(0, sourceLesson.Subgroup),
					Schedule: connector.Schedule{
						Date: date.Format(time.DateOnly), StartsAt: start, EndsAt: end,
						Recurrence: connector.Recurrence{Kind: connector.RecurrenceDate},
					},
					Metadata: map[string]string{
						"source_form":        strings.TrimSpace(sourceLesson.Form),
						"period":             strings.TrimSpace(period.Title),
						"source_week":        strconv.Itoa(sourceLesson.Week),
						"remote":             strconv.FormatBool(sourceLesson.Remote),
						"source_modified_at": period.Modified,
					},
				})
			}
		}
	}
	sort.Slice(lessons, func(i, j int) bool {
		if lessons[i].Schedule.Date != lessons[j].Schedule.Date {
			return lessons[i].Schedule.Date < lessons[j].Schedule.Date
		}
		if lessons[i].Schedule.StartsAt != lessons[j].Schedule.StartsAt {
			return lessons[i].Schedule.StartsAt < lessons[j].Schedule.StartsAt
		}
		return lessons[i].ExternalID < lessons[j].ExternalID
	})
	return lessons, nil
}

func (p *Parser) getJSON(ctx context.Context, path string, target any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := p.wait(ctx); err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL, "/")+path, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "Scheduler-Managed-Parser/1.0 (+ivgpu)")
		response, requestErr := p.client.Do(request)
		if requestErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
			_ = response.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if len(body) > maxResponseBytes {
				lastErr = fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
			} else if err = json.Unmarshal(body, target); err == nil {
				return nil
			} else {
				lastErr = fmt.Errorf("decode JSON: %w", err)
			}
		} else if requestErr != nil {
			lastErr = requestErr
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", response.StatusCode)
			if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
				return lastErr
			}
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func (p *Parser) wait(ctx context.Context) error {
	p.paceMu.Lock()
	now := time.Now()
	when := p.nextHit
	if when.Before(now) {
		when = now
	}
	p.nextHit = when.Add(p.requestInterval)
	p.paceMu.Unlock()
	if delay := time.Until(when); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func lessonTime(times map[string]string, index int) (string, string, error) {
	raw, ok := times[strconv.Itoa(index)]
	if !ok {
		return "", "", fmt.Errorf("lesson time %d is absent", index)
	}
	parts := timeRangePattern.FindStringSubmatch(raw)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("unsupported lesson time %q", raw)
	}
	start, err := normalizeClock(parts[1])
	if err != nil {
		return "", "", err
	}
	end, err := normalizeClock(parts[2])
	if err != nil {
		return "", "", err
	}
	if start >= end {
		return "", "", fmt.Errorf("lesson time %q is not ordered", raw)
	}
	return start, end, nil
}

func normalizeClock(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid time %q", value)
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid time %q", value)
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), nil
}

func lessonType(form string) string {
	normalized := strings.ToUpper(strings.TrimSpace(form))
	switch {
	case strings.Contains(normalized, "ЭКЗ"):
		return "exam"
	case strings.Contains(normalized, "ЗАЧ") || strings.Contains(normalized, "ЗАЧЁТ"):
		return "credit"
	case strings.Contains(normalized, "КОНС"):
		return "consultation"
	case strings.Contains(normalized, "ЛАБ"):
		return "lab"
	case strings.Contains(normalized, "ЛЕК"):
		return "lecture"
	case strings.Contains(normalized, "ПР") || strings.Contains(normalized, "ПРАКТИКА"):
		return "practice"
	default:
		return "seminar"
	}
}

func parseDate(value string) (time.Time, error) {
	for _, layout := range []string{time.DateOnly, "02.01.2006", "02.01.06"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid lesson date %q", value)
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:12])
}
