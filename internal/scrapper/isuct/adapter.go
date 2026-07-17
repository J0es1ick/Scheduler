package isuct

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/PuerkitoBio/goquery"
)

const (
	UniversityID = "isuct"

	defaultBaseURL = "https://www.isuct.ru"
	httpTimeout    = 45 * time.Second

	autocompleteLimit       = 15
	autocompleteConcurrency = 4
	maxHTTPAttempts         = 3
	maxScheduleAttempts     = 3
)

var (
	cssToLessonType = map[string]domain.LessonType{
		"type-lk":   domain.LessonTypeLecture,
		"type-pz":   domain.LessonTypePractice,
		"type-lab":  domain.LessonTypeLab,
		"type-sem":  domain.LessonTypeSeminar,
		"type-kons": domain.LessonTypeSeminar,
	}
	colToDayOfWeek = []int{1, 2, 3, 4, 5, 6}

	reDates   = regexp.MustCompile(`(?i)с\s+(\d{2}\.\d{2}\.\d{4})\s+по\s+(\d{2}\.\d{2}\.\d{4})`)
	reTime    = regexp.MustCompile(`^\s*(\d{2}:\d{2})\s*[-–—]\s*(\d{2}:\d{2})\s*$`)
	reTeacher = regexp.MustCompile(`\s+([А-ЯЁ][а-яёА-ЯЁ-]+(?:\s+[А-ЯЁ]\.[А-ЯЁ]\.)+(?:,\s*[А-ЯЁ][а-яёА-ЯЁ-]+(?:\s+[А-ЯЁ]\.[А-ЯЁ]\.)+)*)$`)
	reSpaces  = regexp.MustCompile(`\s{2,}`)
	reTags    = regexp.MustCompile(`<[^>]+>`)

	typeAbbrevs = []string{" пр.з. ", " лаб. ", " лк. ", " сем. ", " конс. "}

	errEmptyAJAXResponse = errors.New("isuct: empty AJAX response")
)

type Adapter struct {
	client          *http.Client
	baseURL         string
	scheduleURL     string
	autocompleteURL string
	ajaxURL         string

	mu          sync.RWMutex
	semesterID  string
	groupNames  map[string]string
	formBuildID string
}

var _ scrapper.SourceAdapter = (*Adapter)(nil)

func New(semesterID string) *Adapter {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: httpTimeout,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("isuct: too many redirects")
			}
			return nil
		},
	}
	return newAdapter(defaultBaseURL, semesterID, client)
}

func newAdapter(baseURL, semesterID string, client *http.Client) *Adapter {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Adapter{
		client:          client,
		baseURL:         baseURL,
		scheduleURL:     baseURL + "/student/schedule",
		autocompleteURL: baseURL + "/index.php?q=student/schedule/currentstudentsgroups",
		ajaxURL:         baseURL + "/system/ajax",
		semesterID:      semesterID,
		groupNames:      make(map[string]string),
	}
}

func (a *Adapter) SetSemesterID(id string) {
	a.mu.Lock()
	a.semesterID = id
	a.mu.Unlock()
}

func (a *Adapter) Name() string         { return "ИГХТУ" }
func (a *Adapter) UniversityID() string { return UniversityID }

func (a *Adapter) FetchGroups(ctx context.Context) ([]domain.Group, error) {
	frontier := make([]string, 10)
	for i := range frontier {
		frontier[i] = fmt.Sprint(i)
	}
	seenPrefixes := make(map[string]bool, 128)
	for _, prefix := range frontier {
		seenPrefixes[prefix] = true
	}

	discovered := make(map[string]autocompleteItem, 256)
	for len(frontier) > 0 {
		results := a.fetchPrefixBatch(ctx, frontier)
		next := make([]string, 0)
		for _, result := range results {
			if result.err != nil {
				return nil, fmt.Errorf("isuct FetchGroups prefix=%q: %w", result.prefix, result.err)
			}
			for _, item := range result.items {
				if item.Value != "" && item.Label != "" {
					discovered[item.Value] = item
				}
			}
			if len(result.items) < autocompleteLimit {
				continue
			}
			for _, suffix := range "0123456789/" {
				child := result.prefix + string(suffix)
				if !seenPrefixes[child] {
					seenPrefixes[child] = true
					next = append(next, child)
				}
			}
		}
		frontier = next
	}

	if len(discovered) == 0 {
		return nil, errors.New("isuct FetchGroups: autocomplete returned no groups")
	}

	now := time.Now()
	groups := make([]domain.Group, 0, len(discovered))
	groupNames := make(map[string]string, len(discovered))
	for extID, item := range discovered {
		groupNames[extID] = item.Label
		groups = append(groups, domain.Group{
			ID:           groupID(extID),
			UniversityID: UniversityID,
			Name:         item.Label,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })

	a.mu.Lock()
	a.groupNames = groupNames
	a.mu.Unlock()
	return groups, nil
}

type prefixResult struct {
	prefix string
	items  []autocompleteItem
	err    error
}

func (a *Adapter) fetchPrefixBatch(ctx context.Context, prefixes []string) []prefixResult {
	results := make([]prefixResult, len(prefixes))
	sem := make(chan struct{}, autocompleteConcurrency)
	var wg sync.WaitGroup
	for i, prefix := range prefixes {
		i, prefix := i, prefix
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = prefixResult{prefix: prefix, err: ctx.Err()}
				return
			}
			items, err := a.fetchGroupsByPrefix(ctx, prefix)
			results[i] = prefixResult{prefix: prefix, items: items, err: err}
		}()
	}
	wg.Wait()
	return results
}

func (a *Adapter) fetchGroupsByPrefix(ctx context.Context, prefix string) ([]autocompleteItem, error) {
	form := url.Values{"search": []string{prefix}}
	body, err := a.do(ctx, http.MethodPost, a.autocompleteURL, form, func(req *http.Request) {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	})
	if err != nil {
		return nil, err
	}
	items, err := parseAutocompleteResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse autocomplete JSON: %w", err)
	}
	return items, nil
}

type autocompleteItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func parseAutocompleteResponse(body []byte) ([]autocompleteItem, error) {
	body = bytes.TrimPrefix(bytes.TrimSpace(body), []byte{0xEF, 0xBB, 0xBF})
	if len(body) == 0 {
		return nil, errors.New("empty response")
	}

	var arr []autocompleteItem
	if err := json.Unmarshal(body, &arr); err == nil && arr != nil {
		result := make([]autocompleteItem, 0, len(arr))
		for _, item := range arr {
			if normalized, ok := normalizeAutocompleteItem(item.Label, item.Value); ok {
				result = append(result, normalized)
			}
		}
		return result, nil
	}

	var obj map[string]string
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	result := make([]autocompleteItem, 0, len(obj))
	for key, value := range obj {
		if item, ok := normalizeAutocompleteItem(value, key); ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func normalizeAutocompleteItem(label, value string) (autocompleteItem, bool) {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if idx := strings.LastIndex(value, "|"); idx >= 0 {
		left := strings.TrimSpace(value[:idx])
		right := strings.TrimSpace(value[idx+1:])
		if label == "" || strings.Contains(label, "|") {
			label = left
		}
		if right != "" {
			return autocompleteItem{Value: right, Label: label}, label != ""
		}
	}
	if idx := strings.Index(label, "|"); idx >= 0 {
		left := strings.TrimSpace(label[:idx])
		right := strings.TrimSpace(label[idx+1:])
		return autocompleteItem{Value: left, Label: right}, left != "" && right != ""
	}
	if label != "" && value != "" {
		return autocompleteItem{Value: value, Label: label}, true
	}
	return autocompleteItem{}, false
}

func (a *Adapter) FetchSchedule(ctx context.Context, gid string) ([]domain.Lesson, error) {
	extID := extractExtID(gid)
	if extID == "" {
		return nil, fmt.Errorf("isuct FetchSchedule: invalid groupID=%q", gid)
	}

	a.mu.RLock()
	groupName := a.groupNames[extID]
	semesterID := a.semesterID
	a.mu.RUnlock()
	if groupName == "" {
		groupName = extID
	}

	var lastErr error
	for attempt := 1; attempt <= maxScheduleAttempts; attempt++ {
		formBuildID, err := a.getFormBuildID(ctx)
		if err != nil {
			return nil, fmt.Errorf("isuct FetchSchedule: get form token: %w", err)
		}
		fragment, err := a.postScheduleAJAX(ctx, extID, groupName, formBuildID)
		if err == nil {
			lessons, parseErr := parseScheduleTable(fragment, gid, semesterID)
			if parseErr == nil {
				return lessons, nil
			}
			err = fmt.Errorf("parse schedule table: %w", parseErr)
		}
		lastErr = err
		if errors.Is(err, errEmptyAJAXResponse) {
			a.invalidateFormBuildID(formBuildID)
		}
		if attempt < maxScheduleAttempts {
			select {
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("isuct FetchSchedule group=%s: %w", groupName, lastErr)
}

func (a *Adapter) getFormBuildID(ctx context.Context) (string, error) {
	a.mu.RLock()
	token := a.formBuildID
	a.mu.RUnlock()
	if token != "" {
		return token, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.formBuildID != "" {
		return a.formBuildID, nil
	}
	body, err := a.do(ctx, http.MethodGet, a.scheduleURL, nil, nil)
	if err != nil {
		return "", err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	form := doc.Find("#studschedule-form")
	token, _ = form.Find(`input[name="form_build_id"]`).Attr("value")
	formID, _ := form.Find(`input[name="form_id"]`).Attr("value")
	if token == "" || formID != "studschedule_form" {
		return "", errors.New("schedule form token not found")
	}
	a.formBuildID = token
	return token, nil
}

func (a *Adapter) invalidateFormBuildID(token string) {
	a.mu.Lock()
	if a.formBuildID == token {
		a.formBuildID = ""
	}
	a.mu.Unlock()
}

func (a *Adapter) postScheduleAJAX(ctx context.Context, extID, groupName, formBuildID string) (string, error) {
	form := url.Values{}
	form.Set("type", "currentstudentsgroups")
	form.Set("idgr", groupName)
	form.Set("idaud", "")
	form.Set("idprep", "")
	form.Set("idprepid", "")
	form.Set("idaudid", "")
	form.Set("idgrid", extID)
	form.Set("op", "Показать расписание")
	form.Set("form_build_id", formBuildID)
	form.Set("form_id", "studschedule_form")
	form.Set("_triggering_element_name", "op")
	form.Set("_triggering_element_value", "Показать расписание")
	form.Set("ajax_page_state[theme]", "isuct")

	body, err := a.do(ctx, http.MethodPost, a.ajaxURL, form, func(req *http.Request) {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", a.scheduleURL)
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	})
	if err != nil {
		return "", err
	}
	return parseAJAXScheduleFragment(body)
}

type ajaxCommand struct {
	Command string `json:"command"`
	Data    string `json:"data"`
}

func parseAJAXScheduleFragment(body []byte) (string, error) {
	body = bytes.TrimPrefix(bytes.TrimSpace(body), []byte{0xEF, 0xBB, 0xBF})
	var commands []ajaxCommand
	if err := json.Unmarshal(body, &commands); err != nil {
		return "", fmt.Errorf("decode AJAX JSON: %w", err)
	}
	if len(commands) == 0 {
		return "", errEmptyAJAXResponse
	}
	for _, command := range commands {
		if command.Command != "insert" {
			continue
		}
		if strings.Contains(command.Data, `class="schedule"`) {
			return command.Data, nil
		}
		if strings.Contains(command.Data, "form-ajax-node-content") {
			return "", nil
		}
	}
	return "", errors.New("schedule fragment not found in AJAX response")
}

func parseScheduleTable(htmlBody, gid, semesterID string) ([]domain.Lesson, error) {
	if strings.TrimSpace(htmlBody) == "" {
		return nil, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil, err
	}
	table := doc.Find("table.schedule").First()
	if table.Length() == 0 {
		return nil, errors.New("schedule table not found in HTML")
	}
	rows := table.Find("tr")
	if rows.Length() <= 2 {
		return nil, nil
	}

	var lessons []domain.Lesson
	currentWeekType := domain.WeekTypeOdd
	now := time.Now()
	rows.Each(func(rowIndex int, row *goquery.Selection) {
		if rowIndex < 2 {
			return
		}
		cells := row.ChildrenFiltered("td")
		if cells.Length() == 0 {
			return
		}

		cellOffset := 0
		first := cells.Eq(0)
		if _, ok := first.Attr("rowspan"); ok && !first.HasClass("time") {
			switch strings.TrimSpace(first.Text()) {
			case "1":
				currentWeekType = domain.WeekTypeOdd
			case "2":
				currentWeekType = domain.WeekTypeEven
			}
			cellOffset = 1
		}
		if cells.Length() <= cellOffset || !cells.Eq(cellOffset).HasClass("time") {
			return
		}
		tm := reTime.FindStringSubmatch(strings.TrimSpace(cells.Eq(cellOffset).Text()))
		if tm == nil {
			return
		}
		timeStart, timeEnd := tm[1], tm[2]
		for col := 0; col < len(colToDayOfWeek); col++ {
			cellIndex := cellOffset + 1 + col
			if cellIndex >= cells.Length() {
				break
			}
			cell := cells.Eq(cellIndex)
			cssClass := lessonCSSClass(cell)
			if cssClass == "" {
				continue
			}
			subject, teacher, room, validFrom, validTo, ok := parseCell(cellText(cell), cssClass)
			if !ok || subject == "" {
				continue
			}
			lessonType := cssToLessonType[cssClass]
			from, to := validFrom, validTo
			lessons = append(lessons, domain.Lesson{
				ID:           lessonStableID(gid, colToDayOfWeek[col], timeStart, subject, teacher, room, string(currentWeekType), validFrom, validTo),
				UniversityID: UniversityID,
				SemesterID:   semesterID,
				DayOfWeek:    colToDayOfWeek[col],
				TimeStart:    timeStart,
				TimeEnd:      timeEnd,
				WeekType:     currentWeekType,
				Subject:      subject,
				Type:         lessonType,
				Teacher:      teacher,
				Room:         room,
				GroupID:      gid,
				Subgroup:     0,
				ValidFrom:    &from,
				ValidTo:      &to,
				UpdatedAt:    now,
			})
		}
	})
	return lessons, nil
}

func parseCell(rawText, _ string) (subject, teacher, room string, validFrom, validTo time.Time, ok bool) {
	text := reSpaces.ReplaceAllString(strings.TrimSpace(rawText), " ")
	dm := reDates.FindStringSubmatch(text)
	if dm == nil {
		return
	}
	var err error
	if validFrom, err = time.Parse("02.01.2006", dm[1]); err != nil {
		return
	}
	if validTo, err = time.Parse("02.01.2006", dm[2]); err != nil {
		return
	}
	dateIndex := reDates.FindStringIndex(text)
	mainPart := strings.TrimSpace(text[:dateIndex[0]])

	splitIndex, splitLength := -1, 0
	for _, abbreviation := range typeAbbrevs {
		if index := strings.LastIndex(mainPart, abbreviation); index > splitIndex {
			splitIndex, splitLength = index, len(abbreviation)
		}
	}
	if splitIndex < 0 {
		return strings.TrimSpace(mainPart), "", "", validFrom, validTo, true
	}

	beforeType := mainPart[:splitIndex]
	room = strings.TrimSpace(mainPart[splitIndex+splitLength:])
	if match := reTeacher.FindStringSubmatchIndex(beforeType); match != nil {
		subject = strings.TrimSpace(beforeType[:match[0]])
		teacher = strings.TrimSpace(beforeType[match[2]:match[3]])
	} else {
		subject = strings.TrimSpace(beforeType)
	}
	return subject, teacher, room, validFrom, validTo, true
}

func cellText(cell *goquery.Selection) string {
	html, _ := cell.Html()
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "<br>", "\n")
	return strings.TrimSpace(reTags.ReplaceAllString(html, ""))
}

func lessonCSSClass(cell *goquery.Selection) string {
	for class := range cssToLessonType {
		if cell.HasClass(class) {
			return class
		}
	}
	return ""
}

func groupID(extID string) string { return fmt.Sprintf("%s:group:%s", UniversityID, extID) }

func extractExtID(gid string) string {
	parts := strings.SplitN(gid, ":", 3)
	if len(parts) == 3 && parts[0] == UniversityID && parts[1] == "group" {
		return parts[2]
	}
	return ""
}

func lessonStableID(gid string, day int, start, subject, teacher, room, weekType string, validFrom, validTo time.Time) string {
	key := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%s|%s", gid, day, start, subject, teacher, room, weekType, validFrom.Format("2006-01-02"), validTo.Format("2006-01-02"))
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:12])
}

func (a *Adapter) do(ctx context.Context, method, requestURL string, form url.Values, configure func(*http.Request)) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxHTTPAttempts; attempt++ {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
		if err != nil {
			return nil, err
		}
		setCommonHeaders(req)
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		}
		if configure != nil {
			configure(req)
		}
		resp, err := a.client.Do(req)
		if err == nil {
			responseBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode == http.StatusOK {
				return responseBody, nil
			} else {
				err = fmt.Errorf("HTTP %d", resp.StatusCode)
				if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
					return nil, err
				}
			}
		}
		lastErr = err
		if attempt == maxHTTPAttempts {
			break
		}
		delay := time.Duration(attempt) * 300 * time.Millisecond
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ScheduleBot/1.0; +https://github.com/J0es1ick/Scheduler)")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
}
