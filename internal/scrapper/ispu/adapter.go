package ispu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
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

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/PuerkitoBio/goquery"
)

const (
	UniversityID = "ispu"

	defaultBaseURL  = "http://schedule.ispu.ru"
	httpTimeout     = 45 * time.Second
	maxHTTPAttempts = 3

	scheduleControl = "ctl00$ContentPlaceHolder1$ddlSchedule"
	facultyControl  = "ctl00$ContentPlaceHolder1$ddlSubDivision"
	courseControl   = "ctl00$ContentPlaceHolder1$ddlCorse"
	groupControl    = "ctl00$ContentPlaceHolder1$ddlObjectValue"
	subgroupControl = "ctl00$ContentPlaceHolder1$rblSubGroup"
)

var (
	reDate          = regexp.MustCompile(`\b(\d{2}\.\d{2}\.\d{4})\b`)
	reScheduleRange = regexp.MustCompile(`(?i)начало\s*:\s*(\d{2}\.\d{2}\.\d{4})\s*[-–—]\s*окончание\s*:\s*(\d{2}\.\d{2}\.\d{4})`)
	reTime          = regexp.MustCompile(`^\s*(\d{1,2})[\.:](\d{2})\s*[-–—]\s*(\d{1,2})[\.:](\d{2})\s*$`)
	reSpaces        = regexp.MustCompile(`\s+`)
	reLessonMarker  = regexp.MustCompile(`(?i)(?:^|\s)(пр\.\s*з\.|пр\.з\.|пр\.|пз\.|лаб\.|лек\.|лк\.|сем\.|конс\.|экз\.|зач\.)(?:\s|$)`)
	reTeacherPrefix = regexp.MustCompile(`^(([А-ЯЁ][А-Яа-яЁё-]+\s+[А-ЯЁ]\.[А-ЯЁ]\.)(?:\s*,\s*[А-ЯЁ][А-Яа-яЁё-]+\s+[А-ЯЁ]\.[А-ЯЁ]\.)*)(?:\s+(.+))?$`)
	reTeacherTail   = regexp.MustCompile(`\s+(([А-ЯЁ][А-Яа-яЁё-]+\s+[А-ЯЁ]\.[А-ЯЁ]\.)(?:\s*,\s*[А-ЯЁ][А-Яа-яЁё-]+\s+[А-ЯЁ]\.[А-ЯЁ]\.)*)(?:\s+(.+))?$`)
)

type Adapter struct {
	client      *http.Client
	scheduleURL string

	mu         sync.RWMutex
	semesterID string
	groups     map[string]groupDescriptor
}

type groupDescriptor struct {
	name  string
	paths []groupPath
}

type groupPath struct {
	form          url.Values
	selectedPage  *webFormsPage
	scheduleLabel string
	facultyLabel  string
	courseLabel   string
	groupValue    string
}

type webFormsPage struct {
	doc  *goquery.Document
	form url.Values
}

type option struct {
	value    string
	label    string
	selected bool
}

var _ scrapper.SourceAdapter = (*Adapter)(nil)

func New(semesterID string) *Adapter {
	client := &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("ispu: too many redirects")
			}
			return nil
		},
	}
	return newAdapter(defaultBaseURL, semesterID, client)
}

func newAdapter(baseURL, semesterID string, client *http.Client) *Adapter {
	return &Adapter{
		client:      client,
		scheduleURL: strings.TrimRight(baseURL, "/") + "/",
		semesterID:  semesterID,
		groups:      make(map[string]groupDescriptor),
	}
}

func (a *Adapter) Name() string         { return "ИГЭУ" }
func (a *Adapter) UniversityID() string { return UniversityID }

func (a *Adapter) SetSemesterID(id string) {
	a.mu.Lock()
	a.semesterID = id
	a.mu.Unlock()
}

func (a *Adapter) FetchGroups(ctx context.Context) ([]domain.Group, error) {
	root, err := a.getPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("ispu FetchGroups: open schedule: %w", err)
	}

	schedules := selectOptions(root.doc, scheduleControl)
	if len(schedules) == 0 {
		return nil, errors.New("ispu FetchGroups: schedule options not found")
	}

	discovered := make(map[string]groupDescriptor, 256)
	pathKeys := make(map[string]map[string]bool, 256)
	for _, scheduleOption := range schedules {
		schedulePage, err := a.pageForOption(ctx, root, scheduleControl, scheduleOption)
		if err != nil {
			return nil, fmt.Errorf("ispu FetchGroups: select schedule %q: %w", scheduleOption.label, err)
		}
		faculties := selectOptions(schedulePage.doc, facultyControl)
		if len(faculties) == 0 {
			return nil, fmt.Errorf("ispu FetchGroups: no faculties for schedule %q", scheduleOption.label)
		}

		for _, facultyOption := range faculties {
			facultyPage, err := a.pageForOption(ctx, schedulePage, facultyControl, facultyOption)
			if err != nil {
				return nil, fmt.Errorf("ispu FetchGroups: select faculty %q in %q: %w", facultyOption.label, scheduleOption.label, err)
			}
			courses := selectOptions(facultyPage.doc, courseControl)
			for _, courseOption := range courses {
				coursePage, err := a.pageForOption(ctx, facultyPage, courseControl, courseOption)
				if err != nil {
					return nil, fmt.Errorf("ispu FetchGroups: select course %q, faculty %q: %w", courseOption.label, facultyOption.label, err)
				}
				groupOptions := selectOptions(coursePage.doc, groupControl)
				for _, groupOption := range groupOptions {
					extID := strings.TrimSpace(groupOption.value)
					if extID == "" || strings.TrimSpace(groupOption.label) == "" {
						continue
					}
					descriptor := discovered[extID]
					if descriptor.name == "" {
						descriptor.name = displayGroupName(courseOption.label, groupOption.label)
					}
					pathKey := strings.Join([]string{scheduleOption.value, facultyOption.value, courseOption.value}, "|")
					if pathKeys[extID] == nil {
						pathKeys[extID] = make(map[string]bool)
					}
					if !pathKeys[extID][pathKey] {
						pathKeys[extID][pathKey] = true
						var selectedPage *webFormsPage
						if groupOption.selected {
							selectedPage = coursePage
						}
						descriptor.paths = append(descriptor.paths, groupPath{
							form:          cloneValues(coursePage.form),
							selectedPage:  selectedPage,
							scheduleLabel: scheduleOption.label,
							facultyLabel:  facultyOption.label,
							courseLabel:   courseOption.label,
							groupValue:    extID,
						})
					}
					discovered[extID] = descriptor
				}
			}
		}
	}

	if len(discovered) == 0 {
		return nil, errors.New("ispu FetchGroups: no groups found in published schedules")
	}
	makeGroupNamesUnique(discovered)

	now := time.Now()
	groups := make([]domain.Group, 0, len(discovered))
	for extID, descriptor := range discovered {
		groups = append(groups, domain.Group{
			ID:           groupID(extID),
			UniversityID: UniversityID,
			Name:         descriptor.name,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return naturalLess(groups[i].Name, groups[j].Name) })

	a.mu.Lock()
	a.groups = discovered
	a.mu.Unlock()
	return groups, nil
}

func (a *Adapter) pageForOption(ctx context.Context, current *webFormsPage, control string, item option) (*webFormsPage, error) {
	if item.selected {
		return current, nil
	}
	return a.postSelection(ctx, current.form, control, control, item.value)
}

func (a *Adapter) FetchSchedule(ctx context.Context, gid string) ([]domain.Lesson, error) {
	extID := extractExtID(gid)
	if extID == "" {
		return nil, fmt.Errorf("ispu FetchSchedule: invalid groupID=%q", gid)
	}

	a.mu.RLock()
	descriptor, ok := a.groups[extID]
	semesterID := a.semesterID
	a.mu.RUnlock()
	if !ok || len(descriptor.paths) == 0 {
		return nil, fmt.Errorf("ispu FetchSchedule: group %s was not discovered", gid)
	}

	var combined []domain.Lesson
	for _, path := range descriptor.paths {
		var err error
		groupPage := path.selectedPage
		if groupPage == nil {
			groupPage, err = a.postSelection(ctx, path.form, groupControl, groupControl, path.groupValue)
			if err != nil {
				return nil, fmt.Errorf("ispu FetchSchedule group=%s schedule=%q: %w", descriptor.name, path.scheduleLabel, err)
			}
		}

		subgroups := radioOptions(groupPage.doc, subgroupControl)
		if len(subgroups) == 0 {
			lessons, err := parseScheduleTable(groupPage.doc, gid, semesterID, path.scheduleLabel)
			if err != nil {
				return nil, fmt.Errorf("ispu FetchSchedule group=%s: %w", descriptor.name, err)
			}
			combined = append(combined, lessons...)
			continue
		}

		pages := make([][]domain.Lesson, len(subgroups))
		for i, subgroup := range subgroups {
			page := groupPage
			if !subgroup.selected {
				eventTarget := subgroupControl + "$" + strconv.Itoa(i)
				page, err = a.postSelection(ctx, groupPage.form, eventTarget, subgroupControl, subgroup.value)
				if err != nil {
					return nil, fmt.Errorf("ispu FetchSchedule group=%s subgroup=%q: %w", descriptor.name, subgroup.label, err)
				}
			}
			pages[i], err = parseScheduleTable(page.doc, gid, semesterID, path.scheduleLabel)
			if err != nil {
				return nil, fmt.Errorf("ispu FetchSchedule group=%s subgroup=%q: %w", descriptor.name, subgroup.label, err)
			}
		}
		combined = append(combined, mergeSubgroupPages(pages)...)
	}

	combined = deduplicateLessons(combined)
	for i := range combined {
		combined[i].ID = lessonStableID(combined[i])
	}
	sort.SliceStable(combined, func(i, j int) bool {
		leftDate, rightDate := lessonSortDate(combined[i]), lessonSortDate(combined[j])
		if !leftDate.Equal(rightDate) {
			return leftDate.Before(rightDate)
		}
		if combined[i].TimeStart != combined[j].TimeStart {
			return combined[i].TimeStart < combined[j].TimeStart
		}
		return combined[i].Subject < combined[j].Subject
	})
	return combined, nil
}

func (a *Adapter) getPage(ctx context.Context) (*webFormsPage, error) {
	body, err := a.do(ctx, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}
	return parseWebFormsPage(body)
}

func (a *Adapter) postSelection(ctx context.Context, base url.Values, eventTarget, field, value string) (*webFormsPage, error) {
	form := cloneValues(base)
	form.Set("__EVENTTARGET", eventTarget)
	form.Set("__EVENTARGUMENT", "")
	form.Set("__LASTFOCUS", "")
	form.Set(field, value)
	body, err := a.do(ctx, http.MethodPost, form)
	if err != nil {
		return nil, err
	}
	return parseWebFormsPage(body)
}

func parseWebFormsPage(body []byte) (*webFormsPage, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	formSelection := doc.Find("form#form1").First()
	if formSelection.Length() == 0 {
		return nil, errors.New("ASP.NET form not found")
	}
	values := make(url.Values)
	formSelection.Find("input[name]").Each(func(_ int, input *goquery.Selection) {
		if _, disabled := input.Attr("disabled"); disabled {
			return
		}
		name, _ := input.Attr("name")
		typeName := strings.ToLower(strings.TrimSpace(attrOr(input, "type", "text")))
		if (typeName == "radio" || typeName == "checkbox") && !isSelected(input) {
			return
		}
		value := attrOr(input, "value", "")
		if value == "" && (typeName == "radio" || typeName == "checkbox") {
			value = "on"
		}
		values.Add(name, value)
	})
	formSelection.Find("select[name]").Each(func(_ int, selectNode *goquery.Selection) {
		if _, disabled := selectNode.Attr("disabled"); disabled {
			return
		}
		name, _ := selectNode.Attr("name")
		selected := selectNode.Find("option[selected]").First()
		if selected.Length() == 0 {
			selected = selectNode.Find("option").First()
		}
		if selected.Length() > 0 {
			values.Set(name, attrOr(selected, "value", strings.TrimSpace(selected.Text())))
		}
	})
	formSelection.Find("textarea[name]").Each(func(_ int, textarea *goquery.Selection) {
		name, _ := textarea.Attr("name")
		values.Set(name, textarea.Text())
	})
	for _, required := range []string{"__VIEWSTATE", "__VIEWSTATEGENERATOR", "__EVENTVALIDATION"} {
		if values.Get(required) == "" {
			return nil, fmt.Errorf("ASP.NET field %s not found", required)
		}
	}
	return &webFormsPage{doc: doc, form: values}, nil
}

func selectOptions(doc *goquery.Document, control string) []option {
	var result []option
	doc.Find("select").EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		name, _ := selection.Attr("name")
		if name != control {
			return true
		}
		selection.Find("option").Each(func(_ int, item *goquery.Selection) {
			result = append(result, option{
				value:    attrOr(item, "value", strings.TrimSpace(item.Text())),
				label:    normalizeText(item.Text()),
				selected: isSelected(item),
			})
		})
		return false
	})
	return result
}

func radioOptions(doc *goquery.Document, control string) []option {
	var result []option
	doc.Find("input[type=radio]").Each(func(_ int, input *goquery.Selection) {
		name, _ := input.Attr("name")
		if name != control {
			return
		}
		id, _ := input.Attr("id")
		label := ""
		doc.Find("label").EachWithBreak(func(_ int, candidate *goquery.Selection) bool {
			forID, _ := candidate.Attr("for")
			if forID == id {
				label = normalizeText(candidate.Text())
				return false
			}
			return true
		})
		result = append(result, option{
			value:    attrOr(input, "value", ""),
			label:    label,
			selected: isSelected(input),
		})
	})
	return result
}

func parseScheduleTable(doc *goquery.Document, gid, semesterID, scheduleLabel string) ([]domain.Lesson, error) {
	table := doc.Find("table#sheduleTable").First()
	if table.Length() == 0 {
		return nil, nil
	}

	rangeStart, rangeEnd := parseScheduleRange(doc.Text())
	oneOff := isOneOffSchedule(scheduleLabel)
	var currentDates [7]time.Time
	currentWeek := 1
	now := time.Now()
	var lessons []domain.Lesson

	table.ChildrenFiltered("tbody").ChildrenFiltered("tr").Each(func(_ int, row *goquery.Selection) {
		parseScheduleRow(row, gid, semesterID, oneOff, rangeStart, rangeEnd, &currentDates, &currentWeek, now, &lessons)
	})
	if table.ChildrenFiltered("tbody").Length() == 0 {
		table.ChildrenFiltered("tr").Each(func(_ int, row *goquery.Selection) {
			parseScheduleRow(row, gid, semesterID, oneOff, rangeStart, rangeEnd, &currentDates, &currentWeek, now, &lessons)
		})
	}
	return lessons, nil
}

func parseScheduleRow(
	row *goquery.Selection,
	gid, semesterID string,
	oneOff bool,
	rangeStart, rangeEnd time.Time,
	currentDates *[7]time.Time,
	currentWeek *int,
	now time.Time,
	lessons *[]domain.Lesson,
) {
	cells := row.ChildrenFiltered("td")
	if cells.Length() == 0 {
		return
	}

	var dates []time.Time
	cells.Each(func(_ int, cell *goquery.Selection) {
		if match := reDate.FindStringSubmatch(cell.Text()); match != nil {
			if parsed, err := time.Parse("02.01.2006", match[1]); err == nil {
				dates = append(dates, parsed)
			}
		}
	})
	if len(dates) == 7 {
		copy(currentDates[:], dates)
		return
	}

	offset := 0
	firstText := normalizeText(cells.Eq(0).Text())
	if _, hasRowspan := cells.Eq(0).Attr("rowspan"); hasRowspan && (firstText == "1" || firstText == "2") {
		*currentWeek, _ = strconv.Atoi(firstText)
		offset = 1
	}
	if cells.Length() <= offset {
		return
	}
	timeStart, timeEnd, ok := parseTimeRange(cells.Eq(offset).Text())
	if !ok {
		return
	}

	for dayIndex := 0; dayIndex < 7; dayIndex++ {
		cellIndex := offset + 1 + dayIndex
		if cellIndex >= cells.Length() || currentDates[dayIndex].IsZero() {
			continue
		}
		subject, lessonType, teacher, room, ok := parseLessonCell(cells.Eq(cellIndex))
		if !ok || subject == "" {
			continue
		}

		lesson := domain.Lesson{
			UniversityID: UniversityID,
			SemesterID:   semesterID,
			TimeStart:    timeStart,
			TimeEnd:      timeEnd,
			Subject:      subject,
			Type:         lessonType,
			Teacher:      teacher,
			Room:         room,
			GroupID:      gid,
			UpdatedAt:    now,
		}
		cellDate := currentDates[dayIndex]
		if oneOff {
			specialDate := cellDate
			lesson.WeekType = domain.WeekTypeDate
			lesson.SpecialDate = &specialDate
			lesson.ValidFrom = &specialDate
			lesson.ValidTo = &specialDate
		} else {
			lesson.DayOfWeek = weekdayNumber(cellDate)
			lesson.WeekType = domain.WeekTypeOdd
			if *currentWeek == 2 {
				lesson.WeekType = domain.WeekTypeEven
			}
			validFrom := cellDate
			for !rangeStart.IsZero() && validFrom.Before(rangeStart) {
				validFrom = validFrom.AddDate(0, 0, 14)
			}
			validTo := rangeEnd
			if validTo.IsZero() {
				validTo = cellDate.AddDate(0, 0, 13)
			}
			if validFrom.After(validTo) {
				continue
			}
			lesson.ValidFrom = &validFrom
			lesson.ValidTo = &validTo
		}
		*lessons = append(*lessons, lesson)
	}
}

func parseLessonCell(cell *goquery.Selection) (string, domain.LessonType, string, string, bool) {
	text := normalizeText(cell.Text())
	if text == "" {
		return "", "", "", "", false
	}
	style := strings.ToUpper(strings.ReplaceAll(attrOr(cell, "style", ""), " ", ""))
	if strings.Contains(style, "BACKGROUND:#FFFFFF") {
		return "", "", "", "", false
	}

	lessonType := domain.LessonTypeSeminar
	subject := text
	remainder := ""
	matches := reLessonMarker.FindAllStringSubmatchIndex(text, -1)
	if len(matches) > 0 {
		match := matches[len(matches)-1]
		marker := strings.ToLower(strings.ReplaceAll(text[match[2]:match[3]], " ", ""))
		lessonType = lessonTypeForMarker(marker)
		subject = strings.TrimSpace(text[:match[0]])
		remainder = strings.TrimSpace(text[match[1]:])
	}

	teacher, room := "", ""
	if remainder != "" {
		if match := reTeacherPrefix.FindStringSubmatch(remainder); match != nil {
			teacher = normalizeText(match[1])
			room = normalizeText(match[3])
		} else {
			room = remainder
		}
	} else if match := reTeacherTail.FindStringSubmatchIndex(subject); match != nil {
		teacher = normalizeText(subject[match[2]:match[3]])
		if match[6] >= 0 {
			room = normalizeText(subject[match[6]:match[7]])
		}
		subject = strings.TrimSpace(subject[:match[0]])
	}
	return subject, lessonType, teacher, room, subject != ""
}

func lessonTypeForMarker(marker string) domain.LessonType {
	switch marker {
	case "лек.", "лк.":
		return domain.LessonTypeLecture
	case "пр.", "пр.з.", "пз.":
		return domain.LessonTypePractice
	case "лаб.":
		return domain.LessonTypeLab
	case "конс.":
		return domain.LessonTypeConsultation
	case "экз.":
		return domain.LessonTypeExam
	case "зач.":
		return domain.LessonTypeCredit
	default:
		return domain.LessonTypeSeminar
	}
}

func mergeSubgroupPages(pages [][]domain.Lesson) []domain.Lesson {
	if len(pages) <= 1 {
		if len(pages) == 0 {
			return nil
		}
		return pages[0]
	}

	type occurrence struct {
		lesson    domain.Lesson
		subgroups map[int]bool
	}
	byKey := make(map[string]*occurrence)
	for subgroupIndex, lessons := range pages {
		seenOnPage := make(map[string]bool)
		for _, lesson := range lessons {
			key := lessonCoreKey(lesson)
			if seenOnPage[key] {
				continue
			}
			seenOnPage[key] = true
			item := byKey[key]
			if item == nil {
				item = &occurrence{lesson: lesson, subgroups: make(map[int]bool)}
				byKey[key] = item
			}
			item.subgroups[subgroupIndex+1] = true
		}
	}

	result := make([]domain.Lesson, 0, len(byKey))
	for _, item := range byKey {
		if len(item.subgroups) == len(pages) {
			item.lesson.Subgroup = 0
			result = append(result, item.lesson)
			continue
		}
		for subgroup := range item.subgroups {
			lesson := item.lesson
			lesson.Subgroup = subgroup
			result = append(result, lesson)
		}
	}
	return result
}

func deduplicateLessons(lessons []domain.Lesson) []domain.Lesson {
	seen := make(map[string]bool, len(lessons))
	result := make([]domain.Lesson, 0, len(lessons))
	for _, lesson := range lessons {
		key := lessonCoreKey(lesson) + "|" + strconv.Itoa(lesson.Subgroup)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, lesson)
	}
	return result
}

func lessonCoreKey(lesson domain.Lesson) string {
	specialDate := ""
	if lesson.SpecialDate != nil {
		specialDate = lesson.SpecialDate.Format("2006-01-02")
	}
	validFrom, validTo := "", ""
	if lesson.ValidFrom != nil {
		validFrom = lesson.ValidFrom.Format("2006-01-02")
	}
	if lesson.ValidTo != nil {
		validTo = lesson.ValidTo.Format("2006-01-02")
	}
	return strings.Join([]string{
		lesson.GroupID, strconv.Itoa(lesson.DayOfWeek), specialDate,
		lesson.TimeStart, lesson.TimeEnd, string(lesson.WeekType), lesson.Subject,
		string(lesson.Type), lesson.Teacher, lesson.Room, validFrom, validTo,
	}, "|")
}

func lessonStableID(lesson domain.Lesson) string {
	key := lessonCoreKey(lesson) + "|" + strconv.Itoa(lesson.Subgroup)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:12])
}

func parseScheduleRange(text string) (time.Time, time.Time) {
	text = normalizeText(text)
	match := reScheduleRange.FindStringSubmatch(text)
	if match == nil {
		return time.Time{}, time.Time{}
	}
	start, startErr := time.Parse("02.01.2006", match[1])
	end, endErr := time.Parse("02.01.2006", match[2])
	if startErr != nil || endErr != nil || end.Before(start) {
		return time.Time{}, time.Time{}
	}
	return start, end
}

func isOneOffSchedule(label string) bool {
	label = strings.ToLower(normalizeText(label))
	for _, keyword := range []string{"экзам", "зач", "фзво", "сесси", "аттест"} {
		if strings.Contains(label, keyword) {
			return true
		}
	}
	return false
}

func parseTimeRange(text string) (string, string, bool) {
	match := reTime.FindStringSubmatch(normalizeText(text))
	if match == nil {
		return "", "", false
	}
	startHour, _ := strconv.Atoi(match[1])
	endHour, _ := strconv.Atoi(match[3])
	return fmt.Sprintf("%02d:%s", startHour, match[2]), fmt.Sprintf("%02d:%s", endHour, match[4]), true
}

func displayGroupName(course, group string) string {
	course = normalizeText(course)
	group = normalizeText(group)
	if course == "" || strings.HasPrefix(group, course+"-") || strings.HasPrefix(group, course+"/") {
		return group
	}
	return course + "-" + group
}

func makeGroupNamesUnique(groups map[string]groupDescriptor) {
	byName := make(map[string][]string)
	for extID, descriptor := range groups {
		byName[descriptor.name] = append(byName[descriptor.name], extID)
	}
	for name, ids := range byName {
		if len(ids) <= 1 {
			continue
		}
		for _, extID := range ids {
			descriptor := groups[extID]
			faculty := ""
			if len(descriptor.paths) > 0 {
				faculty = descriptor.paths[0].facultyLabel
			}
			if faculty == "" {
				faculty = extID
			}
			descriptor.name = fmt.Sprintf("%s (%s)", name, faculty)
			groups[extID] = descriptor
		}
	}
}

func naturalLess(left, right string) bool {
	leftParts := strings.FieldsFunc(left, func(r rune) bool { return r == '-' || r == '/' })
	rightParts := strings.FieldsFunc(right, func(r rune) bool { return r == '-' || r == '/' })
	for i := 0; i < len(leftParts) && i < len(rightParts); i++ {
		leftNumber, leftErr := strconv.Atoi(leftParts[i])
		rightNumber, rightErr := strconv.Atoi(rightParts[i])
		if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
		if leftParts[i] != rightParts[i] {
			return leftParts[i] < rightParts[i]
		}
	}
	return left < right
}

func lessonSortDate(lesson domain.Lesson) time.Time {
	if lesson.SpecialDate != nil {
		return *lesson.SpecialDate
	}
	if lesson.ValidFrom != nil {
		return *lesson.ValidFrom
	}
	return time.Time{}
}

func weekdayNumber(date time.Time) int {
	weekday := int(date.Weekday())
	if weekday == 0 {
		return 7
	}
	return weekday
}

func groupID(extID string) string { return fmt.Sprintf("%s:group:%s", UniversityID, extID) }

func extractExtID(gid string) string {
	parts := strings.SplitN(gid, ":", 3)
	if len(parts) == 3 && parts[0] == UniversityID && parts[1] == "group" {
		return parts[2]
	}
	return ""
}

func normalizeText(value string) string {
	return strings.TrimSpace(reSpaces.ReplaceAllString(value, " "))
}

func cloneValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, entries := range values {
		clone[key] = append([]string(nil), entries...)
	}
	return clone
}

func attrOr(selection *goquery.Selection, name, fallback string) string {
	if value, ok := selection.Attr(name); ok {
		return value
	}
	return fallback
}

func isSelected(selection *goquery.Selection) bool {
	if _, ok := selection.Attr("selected"); ok {
		return true
	}
	_, ok := selection.Attr("checked")
	return ok
}

func (a *Adapter) do(ctx context.Context, method string, form url.Values) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxHTTPAttempts; attempt++ {
		var requestBody io.Reader
		if form != nil {
			requestBody = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, a.scheduleURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ScheduleBot/1.0; +https://github.com/J0es1ick/Scheduler)")
		req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", strings.TrimRight(a.scheduleURL, "/"))
			req.Header.Set("Referer", a.scheduleURL)
		}

		resp, requestErr := a.client.Do(req)
		if requestErr == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				requestErr = readErr
			} else if resp.StatusCode == http.StatusOK {
				return body, nil
			} else {
				requestErr = fmt.Errorf("HTTP %d", resp.StatusCode)
				if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
					return nil, requestErr
				}
			}
		}
		lastErr = requestErr
		if attempt == maxHTTPAttempts {
			break
		}
		select {
		case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}
