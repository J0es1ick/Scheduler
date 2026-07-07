// Package isuct реализует SourceAdapter для ИГХТУ (ИГХТУ, Иваново).
//
// Механизм: обычная HTML-форма POST /student/schedule (Drupal).
// Никакого отдельного JSON API нет — расписание возвращается прямо внутри
// HTML страницы в виде таблицы <table class="schedule">.
//
// FetchGroups  — GET автокомплит-endpoint, возвращает JSON-список групп.
// FetchSchedule — POST формы с idgrid=<внутренний_id> → парсинг таблицы.
package isuct

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/PuerkitoBio/goquery"
)

const (
	// UniversityID — slug, совпадающий с domain.University.ID в БД.
	UniversityID = "isuct"

	baseURL      = "https://www.isuct.ru"
	scheduleURL  = baseURL + "/student/schedule"
	autocomplURL = baseURL + "/index.php"

	httpTimeout = 30 * time.Second
)

// Маппинг CSS-класса ячейки → domain.LessonType
var cssToLessonType = map[string]domain.LessonType{
	"type-lk":   domain.LessonTypeLecture,
	"type-pz":   domain.LessonTypePractice,
	"type-lab":  domain.LessonTypeLab,
	"type-sem":  domain.LessonTypeSeminar,
	"type-kons": domain.LessonTypeSeminar, // консультация → семинар как ближайший тип
}

// Порядок столбцов в таблице: после ячеек "нед" и "Время" идут Пн=1 … Сб=6.
var colToDayOfWeek = []int{1, 2, 3, 4, 5, 6}

var (
	reDates   = regexp.MustCompile(`с\s+(\d{2}\.\d{2}\.\d{4})\s+по\s+(\d{2}\.\d{2}\.\d{4})`)
	reTime    = regexp.MustCompile(`^(\d{2}:\d{2})-(\d{2}:\d{2})$`)
	reTeacher = regexp.MustCompile(`\s+([А-ЯЁ][а-яёА-ЯЁ]+(?:\s+[А-ЯЁ]\.[А-ЯЁ]\.)+(?:,\s*[А-ЯЁ][а-яёА-ЯЁ]+(?:\s+[А-ЯЁ]\.[А-ЯЁ]\.)+)*)$`)
	reSpaces  = regexp.MustCompile(`\s{2,}`)
	reTags    = regexp.MustCompile(`<[^>]+>`)
)

// typeAbbrevs — разделители типа занятия внутри текста ячейки.
// Порядок важен: более длинные проверяются первыми.
var typeAbbrevs = []string{" пр.з. ", " лаб. ", " лк. ", " сем. ", " конс. "}

// ─────────────────────────────────────────────────────────────
// Adapter
// ─────────────────────────────────────────────────────────────

// Adapter реализует scrapper.SourceAdapter для ИГХТУ.
type Adapter struct {
	client     *http.Client
	semesterID string
}

var _ scrapper.SourceAdapter = (*Adapter)(nil)

// New создаёт адаптер. semesterID — UUID семестра из таблицы semesters,
// к которому будут привязаны занятия.
func New(semesterID string) *Adapter {
	return &Adapter{
		semesterID: semesterID,
		client: &http.Client{
			Timeout: httpTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("isuct: too many redirects")
				}
				return nil
			},
		},
	}
}

func (a *Adapter) SetSemesterID(id string) { a.semesterID = id }
func (a *Adapter) Name() string            { return "ИГХТУ" }
func (a *Adapter) UniversityID() string    { return UniversityID }

// ─────────────────────────────────────────────────────────────
// FetchGroups
// ─────────────────────────────────────────────────────────────

// autocompleteItem — одна запись из JSON-ответа Drupal autocomplete.
// Drupal возвращает объект {"<label>": "<value>"} либо массив
// [{value, label}] — сайт ИГХТУ использует формат объекта.
type autocompleteItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FetchGroups запрашивает полный список групп через autocomplete-endpoint.
// Передаём пустой term — Drupal возвращает все записи (или первые N).
// Если сервер ограничивает выдачу, используйте FetchGroupsByPrefix.
func (a *Adapter) FetchGroups(ctx context.Context) ([]domain.Group, error) {
	return a.FetchGroupsByPrefix(ctx, "")
}

// FetchGroupsByPrefix возвращает группы, имя которых начинается на prefix.
// Полезно для постепенной загрузки большого справочника.
func (a *Adapter) FetchGroupsByPrefix(ctx context.Context, prefix string) ([]domain.Group, error) {
	params := url.Values{}
	params.Set("q", "student/schedule/currentstudentsgroups")
	params.Set("term", prefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		autocomplURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchGroups: build request: %w", err)
	}
	setCommonHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchGroups: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("isuct FetchGroups: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchGroups: read body: %w", err)
	}

	// Drupal 7 autocomplete: {"Название": "id|Название", ...}
	// или массив [{value, label}] — пробуем оба варианта.
	groups, err := parseAutocompleteResponse(body)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchGroups: parse JSON: %w", err)
	}

	now := time.Now()
	result := make([]domain.Group, 0, len(groups))
	for _, item := range groups {
		if item.Value == "" || item.Label == "" {
			continue
		}
		result = append(result, domain.Group{
			ID:           groupID(item.Value),
			UniversityID: UniversityID,
			Name:         item.Label,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	return result, nil
}

// parseAutocompleteResponse обрабатывает оба формата Drupal autocomplete.
func parseAutocompleteResponse(body []byte) ([]autocompleteItem, error) {
	// Попытка 1: массив [{value, label}]
	var arr []autocompleteItem
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	// Попытка 2: объект {"label": "value|label", ...} (Drupal 7)
	var obj map[string]string
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	result := make([]autocompleteItem, 0, len(obj))
	for label, value := range obj {
		// Drupal иногда кодирует value как "extID|DisplayText"
		extID := value
		if idx := strings.Index(value, "|"); idx >= 0 {
			extID = value[:idx]
		}
		result = append(result, autocompleteItem{Value: extID, Label: label})
	}
	return result, nil
}

// ─────────────────────────────────────────────────────────────
// FetchSchedule
// ─────────────────────────────────────────────────────────────

// FetchSchedule загружает и парсит расписание группы.
// groupID здесь имеет вид "isuct:group:<extID>" (значение поля idgrid на сайте).
func (a *Adapter) FetchSchedule(ctx context.Context, gid string) ([]domain.Lesson, error) {
	extID := extractExtID(gid)
	if extID == "" {
		return nil, fmt.Errorf("isuct FetchSchedule: невалидный groupID=%q", gid)
	}

	// Шаг 1: получаем начальную страницу для form_build_id и form_id.
	formBuildID, formID, err := a.fetchFormTokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchSchedule: get form tokens: %w", err)
	}

	// Шаг 2: POST формы с idgrid=extID.
	htmlBody, err := a.postScheduleForm(ctx, extID, formBuildID, formID)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchSchedule group=%s: POST: %w", extID, err)
	}

	// Шаг 3: парсинг HTML-таблицы расписания.
	lessons, err := parseScheduleTable(htmlBody, gid, a.semesterID)
	if err != nil {
		return nil, fmt.Errorf("isuct FetchSchedule group=%s: parse: %w", extID, err)
	}
	return lessons, nil
}

// fetchFormTokens загружает страницу расписания и извлекает скрытые поля
// form_build_id и form_id, без которых Drupal отвергает POST.
func (a *Adapter) fetchFormTokens(ctx context.Context) (formBuildID, formID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheduleURL, nil)
	if err != nil {
		return "", "", err
	}
	setCommonHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", "", err
	}

	formBuildID, _ = doc.Find(`input[name="form_build_id"]`).Attr("value")
	formID, _ = doc.Find(`input[name="form_id"]`).Attr("value")
	if formBuildID == "" || formID == "" {
		return "", "", fmt.Errorf("form tokens not found on page")
	}
	return formBuildID, formID, nil
}

// postScheduleForm отправляет форму и возвращает HTML-ответ.
func (a *Adapter) postScheduleForm(ctx context.Context, extID, formBuildID, formID string) (string, error) {
	form := url.Values{}
	form.Set("type", "currentstudentsgroups")
	form.Set("idgr", "")    // текстовое поле — оставляем пустым, главное idgrid
	form.Set("idaud", "")
	form.Set("idprep", "")
	form.Set("idprepid", "")
	form.Set("idaudid", "")
	form.Set("idgrid", extID)
	form.Set("op", "Показать расписание")
	form.Set("form_build_id", formBuildID)
	form.Set("form_id", formID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scheduleURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", scheduleURL)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ─────────────────────────────────────────────────────────────
// parseScheduleTable
// ─────────────────────────────────────────────────────────────

// parseScheduleTable разбирает HTML-страницу/фрагмент и возвращает занятия.
//
// Структура таблицы:
//   - Строка 0: шапка (нед | Время | Занятия colspan=6)
//   - Строка 1: заголовки дней (Понедельник … Суббота)
//   - Блок недели: первая строка содержит <td rowspan=N> с номером недели,
//     затем <td class="time"> и 6 ячеек дней. Следующие N-1 строк — только
//     ячейка времени + 6 ячеек (без ячейки номера недели).
//   - Блок 1 → нечётная неделя (WeekTypeOdd)
//   - Блок 2 → чётная неделя (WeekTypeEven)
func parseScheduleTable(htmlBody, gid, semesterID string) ([]domain.Lesson, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil, err
	}

	table := doc.Find("table.schedule")
	if table.Length() == 0 {
		return nil, fmt.Errorf("таблица расписания не найдена в HTML")
	}

	rows := table.Find("tbody tr")
	headerRows := 2 // первые две строки — шапка

	// Определяем структуру блоков недель.
	// Первый блок с rowspan → нечётная, второй → чётная.
	weekSeq := []domain.WeekType{domain.WeekTypeOdd, domain.WeekTypeEven}
	type block struct {
		weekType domain.WeekType
		rowCount int
	}
	var blocks []block
	weekIdx := 0

	rows.Slice(headerRows, rows.Length()).Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		first := cells.First()
		if rs, ok := first.Attr("rowspan"); ok {
			n := 0
			fmt.Sscanf(rs, "%d", &n)
			if n > 0 && weekIdx < len(weekSeq) {
				blocks = append(blocks, block{weekType: weekSeq[weekIdx], rowCount: n})
				weekIdx++
			}
		}
	})

	now := time.Now()
	var lessons []domain.Lesson

	rowIdx := headerRows
	for _, blk := range blocks {
		for r := 0; r < blk.rowCount; r++ {
			row := rows.Eq(rowIdx)
			rowIdx++

			var timeStart, timeEnd string
			var dayCells []*goquery.Selection

			row.Find("td").Each(func(_ int, cell *goquery.Selection) {
				switch {
				case cell.HasClass("week"):
					// ячейка номера недели — пропускаем
				case cell.HasClass("time"):
					tm := reTime.FindStringSubmatch(strings.TrimSpace(cell.Text()))
					if tm != nil {
						timeStart, timeEnd = tm[1], tm[2]
					}
				default:
					dayCells = append(dayCells, cell)
				}
			})

			if timeStart == "" || timeEnd == "" {
				continue
			}

			for colIdx, cell := range dayCells {
				if colIdx >= 6 {
					break
				}
				dayOfWeek := colToDayOfWeek[colIdx]

				cssClass := lessonCSSClass(cell)
				if cssClass == "" {
					continue // пустая ячейка
				}

				rawText := cellText(cell)
				// dateFrom/dateTo из сайта — диапазон действия пары в семестре.
				// В domain.Lesson они не хранятся отдельно: принадлежность семестру
				// определяется через SemesterID, чётность — через WeekType (odd/even).
				// Поля сохранены в parseCell для возможного будущего использования
				// (например, фильтрация пар с особым диапазоном дат).
				subj, teacher, room, _, _, ok := parseCell(rawText, cssClass)
				if !ok {
					continue
				}

				lessonType := cssToLessonType[cssClass]
				if lessonType == "" {
					lessonType = domain.LessonTypeLecture
				}

				l := domain.Lesson{
					ID:           lessonStableID(gid, dayOfWeek, timeStart, subj, teacher, string(blk.weekType)),
					UniversityID: UniversityID,
					SemesterID:   semesterID,
					DayOfWeek:    dayOfWeek,
					SpecialDate:  nil,
					TimeStart:    timeStart,
					TimeEnd:      timeEnd,
					WeekType:     blk.weekType,
					Subject:      subj,
					Type:         lessonType,
					Teacher:      teacher,
					Room:         room,
					GroupID:      gid,
					Subgroup:     0,
					UpdatedAt:    now,
				}
				lessons = append(lessons, l)
			}
		}
	}

	return lessons, nil
}

// ─────────────────────────────────────────────────────────────
// Helpers: parseCell, cellText, lessonCSSClass
// ─────────────────────────────────────────────────────────────

// parseCell разбирает текст непустой ячейки занятия.
// Формат: "Предмет Преподаватель тип. Аудитория \nс DD.MM.YYYY по DD.MM.YYYY"
func parseCell(rawText, cssClass string) (subject, teacher, room string, dateFrom, dateTo time.Time, ok bool) {
	text := reSpaces.ReplaceAllString(strings.TrimSpace(rawText), " ")

	dm := reDates.FindStringSubmatch(text)
	if dm == nil {
		return
	}
	var err error
	if dateFrom, err = time.Parse("02.01.2006", dm[1]); err != nil {
		return
	}
	if dateTo, err = time.Parse("02.01.2006", dm[2]); err != nil {
		return
	}

	mainPart := strings.TrimSpace(text[:reDates.FindStringIndex(text)[0]])

	// Ищем последний разделитель типа занятия
	splitIdx := -1
	splitLen := 0
	for _, abbrev := range typeAbbrevs {
		idx := strings.LastIndex(mainPart, abbrev)
		if idx > splitIdx {
			splitIdx = idx
			splitLen = len(abbrev)
		}
	}
	if splitIdx < 0 {
		// Нет разделителя типа — предмет целиком, остальное пусто
		subject = strings.TrimSpace(mainPart)
		ok = true
		return
	}

	beforeType := mainPart[:splitIdx]
	room = strings.TrimSpace(mainPart[splitIdx+splitLen:])

	// Разделяем предмет и преподавателя
	tm := reTeacher.FindStringSubmatchIndex(beforeType)
	if tm != nil {
		subject = strings.TrimSpace(beforeType[:tm[0]])
		teacher = strings.TrimSpace(beforeType[tm[2]:tm[3]])
	} else {
		subject = strings.TrimSpace(beforeType)
	}

	ok = true
	return
}

// cellText возвращает текст ячейки, восстанавливая <br> как перевод строки.
func cellText(cell *goquery.Selection) string {
	html, _ := cell.Html()
	text := strings.ReplaceAll(html, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = reTags.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// lessonCSSClass возвращает CSS-класс типа занятия или "" для пустой ячейки.
func lessonCSSClass(cell *goquery.Selection) string {
	for cls := range cssToLessonType {
		if cell.HasClass(cls) {
			return cls
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────
// ID helpers
// ─────────────────────────────────────────────────────────────

// groupID строит составной ID группы из внешнего idgrid сайта.
func groupID(extID string) string {
	return fmt.Sprintf("%s:group:%s", UniversityID, extID)
}

// extractExtID достаёт числовой idgrid из "isuct:group:<extID>".
func extractExtID(gid string) string {
	parts := strings.SplitN(gid, ":", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return ""
}

// lessonStableID генерирует детерминированный ID занятия.
// Повторный парсинг той же группы даёт те же ID → безопасный upsert.
func lessonStableID(gid string, dayOfWeek int, timeStart, subject, teacher, weekType string) string {
	key := fmt.Sprintf("%s|%d|%s|%s|%s|%s", gid, dayOfWeek, timeStart, subject, teacher, weekType)
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:8])
}

func setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; ScheduleBot/1.0; +https://github.com/J0es1ick/Scheduler)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
}
