package ispu

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/PuerkitoBio/goquery"
)

const maxScheduleRows = 1024

var weekdayHeadings = [...]string{"понедельник", "вторник", "среда", "четверг", "пятница", "суббота", "воскресенье"}

type schedulePeriod struct {
	start, end time.Time
	startWeek  int
}

type scheduleCell struct {
	node    *goquery.Selection
	row     int
	rowspan int
}

type scheduleRow struct {
	cells         [9]*scheduleCell
	dates         [7]time.Time
	week          int
	start, end    string
	explicitDates bool
}

func parseSchedulePeriod(text string) (schedulePeriod, error) {
	text = strings.ToLower(normalizeText(text))
	match := reScheduleRange.FindStringSubmatch(text)
	if match == nil {
		return schedulePeriod{}, nil
	}
	start, startErr := time.Parse("02.01.2006", match[1])
	end, endErr := time.Parse("02.01.2006", match[3])
	if startErr != nil || endErr != nil || end.Before(start) {
		return schedulePeriod{}, errors.New("invalid published schedule period")
	}
	period := schedulePeriod{start: start, end: end}
	if week := reStartWeek.FindStringSubmatch(match[2]); week != nil {
		period.startWeek, _ = strconv.Atoi(week[1])
	}
	return period, nil
}

func expandScheduleTable(table *goquery.Selection) ([]scheduleRow, error) {
	selections := table.Find("tr").FilterFunction(func(_ int, row *goquery.Selection) bool {
		return row.Closest("table").Get(0) == table.Get(0)
	})
	if selections.Length() > maxScheduleRows {
		return nil, errors.New("schedule row limit exceeded")
	}
	rows := make([]scheduleRow, selections.Length())
	var active [9]*scheduleCell
	for r := range rows {
		for column, cell := range active {
			if cell != nil && r < cell.row+cell.rowspan {
				rows[r].cells[column] = cell
			}
		}
		cells := selections.Eq(r).ChildrenFiltered("td, th")
		column := 0
		for c := 0; c < cells.Length(); c++ {
			for column < len(active) && rows[r].cells[column] != nil {
				column++
			}
			node := cells.Eq(c)
			colspan, colErr := strconv.Atoi(attrOr(node, "colspan", "1"))
			rowspan, rowErr := strconv.Atoi(attrOr(node, "rowspan", "1"))
			if colErr != nil || rowErr != nil || colspan < 1 || column+colspan > len(active) || rowspan < 1 || rowspan > maxScheduleRows {
				return nil, fmt.Errorf("invalid cell span at row %d", r+1)
			}
			cell := &scheduleCell{node: node, row: r, rowspan: rowspan}
			for end := column + colspan; column < end; column++ {
				if rows[r].cells[column] != nil {
					return nil, fmt.Errorf("overlapping cells at row %d", r+1)
				}
				rows[r].cells[column] = cell
				active[column] = cell
			}
		}
	}
	return rows, nil
}

func readScheduleDates(rows []scheduleRow, period schedulePeriod) error {
	var datedHeader [7]time.Time
	datedWeek := 0
	timedRows := 0
	explicitEmpty := false
	for r := range rows {
		row := &rows[r]
		start, end, timed := parseTimeRange(scheduleCellText(row.cells[1]))
		if !timed {
			var dates [7]time.Time
			dateCount := 0
			for day := range dates {
				text := strings.ToLower(scheduleCellText(row.cells[day+2]))
				if match := reDate.FindStringSubmatch(text); match != nil {
					date, err := time.Parse("02.01.2006", match[1])
					if err != nil || weekdayNumber(date) != day+1 || (day > 0 && !date.Equal(dates[day-1].AddDate(0, 0, 1))) {
						return fmt.Errorf("invalid day dates at row %d", r+1)
					}
					dates[day] = date
					dateCount++
				}
				for _, heading := range weekdayHeadings {
					if strings.HasPrefix(text, heading) && !strings.HasPrefix(text, weekdayHeadings[day]) {
						return fmt.Errorf("unexpected weekday column at row %d", r+1)
					}
				}
			}
			if dateCount == 7 {
				datedHeader = dates
				datedWeek = 0
				continue
			}
			if dateCount > 0 {
				return fmt.Errorf("incomplete day dates at row %d", r+1)
			}
			for _, cell := range row.cells {
				text := strings.ToLower(scheduleCellText(cell))
				switch text {
				case "", "нед", "время", "занятия":
					continue
				case "занятий нет", "расписание отсутствует", "нет расписания", "расписание не найдено":
					explicitEmpty = true
					continue
				}
				isDay := false
				for _, heading := range weekdayHeadings {
					isDay = isDay || text == heading
				}
				if !isDay {
					return fmt.Errorf("unrecognized schedule row %d (missing or invalid lesson time)", r+1)
				}
			}
			continue
		}
		week, err := strconv.Atoi(scheduleCellText(row.cells[0]))
		if err != nil || (week != 1 && week != 2) {
			return fmt.Errorf("missing week number at row %d", r+1)
		}
		row.week, row.start, row.end = week, start, end
		timedRows++
		if !datedHeader[0].IsZero() {
			if datedWeek != 0 && datedWeek != week {
				for day := range datedHeader {
					datedHeader[day] = datedHeader[day].AddDate(0, 0, 7)
				}
			}
			datedWeek = week
			row.dates, row.explicitDates = datedHeader, true
		} else {
			if period.start.IsZero() || period.startWeek == 0 {
				return fmt.Errorf("cannot determine dates at row %d: missing dated header or period start week", r+1)
			}
			monday := period.start.AddDate(0, 0, 1-weekdayNumber(period.start)+(week-period.startWeek)*7)
			for day := range row.dates {
				row.dates[day] = monday.AddDate(0, 0, day)
			}
		}
		for day := range row.dates {
			if row.cells[day+2] == nil {
				return fmt.Errorf("missing day column at row %d", r+1)
			}
		}
	}
	if timedRows == 0 && !explicitEmpty {
		return errors.New("schedule table contains no recognized lesson rows or explicit empty status")
	}
	return nil
}

func parseScheduleGrid(table *goquery.Selection, period schedulePeriod, gid, semesterID string, oneOff bool) ([]domain.Lesson, error) {
	rows, err := expandScheduleTable(table)
	if err != nil {
		return nil, err
	}
	if err := readScheduleDates(rows, period); err != nil {
		return nil, err
	}
	var lessons []domain.Lesson
	now := time.Now()
	for r, row := range rows {
		if row.start == "" {
			continue
		}
		for day, date := range row.dates {
			cell := row.cells[day+2]
			if cell.row != r {
				continue
			}
			texts := lessonCellTexts(cell)
			if len(texts) == 0 {
				continue
			}
			last := r + cell.rowspan - 1
			if last >= len(rows) {
				return nil, fmt.Errorf("lesson extends beyond the table at row %d", r+1)
			}
			for next := r + 1; next <= last; next++ {
				if rows[next].start == "" || !rows[next].dates[day].Equal(date) || rows[next].week != row.week || rows[next].start < rows[next-1].end {
					return nil, fmt.Errorf("lesson spans incompatible time rows at row %d", r+1)
				}
			}
			if oneOff && !row.explicitDates && period.end.After(period.start.AddDate(0, 0, 13)) {
				return nil, errors.New("one-off schedule requires explicit dates for a period longer than two weeks")
			}
			if !oneOff || !row.explicitDates {
				for !period.start.IsZero() && date.Before(period.start) {
					date = date.AddDate(0, 0, 14)
				}
			}
			if (!period.start.IsZero() && date.Before(period.start)) || (!period.end.IsZero() && date.After(period.end)) {
				continue
			}
			for _, text := range texts {
				subject, lessonType, teacher, room, ok := parseLessonText(text)
				if !ok {
					return nil, fmt.Errorf("unrecognized lesson at row %d", r+1)
				}
				lesson := domain.Lesson{
					UniversityID: UniversityID, SemesterID: semesterID, GroupID: gid,
					TimeStart: row.start, TimeEnd: rows[last].end,
					Subject: subject, Type: lessonType, Teacher: teacher, Room: room, UpdatedAt: now,
				}
				from, to := date, period.end
				if oneOff {
					lesson.WeekType = domain.WeekTypeDate
					lesson.SpecialDate = &from
					to = from
				} else {
					lesson.DayOfWeek = day + 1
					lesson.WeekType = domain.WeekTypeOdd
					if row.week == 2 {
						lesson.WeekType = domain.WeekTypeEven
					}
					if to.IsZero() {
						to = date.AddDate(0, 0, 13)
					}
				}
				lesson.ValidFrom, lesson.ValidTo = &from, &to
				lessons = append(lessons, lesson)
			}
		}
	}
	return lessons, nil
}

func scheduleCellText(cell *scheduleCell) string {
	if cell == nil {
		return ""
	}
	return normalizeText(cell.node.Text())
}

func lessonCellTexts(cell *scheduleCell) []string {
	text := scheduleCellText(cell)
	switch text {
	case "", "-", "—", "–":
		return nil
	}
	parts := strings.Split(text, ";")
	for _, part := range parts {
		if !reLessonMarker.MatchString(part) {
			return []string{text}
		}
	}
	return parts
}
