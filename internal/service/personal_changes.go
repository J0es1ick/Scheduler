package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
)

var explicitBuilding = regexp.MustCompile(`(?i)(?:корпус|корп\.?|к\.)\s*([\p{L}\d-]+)`)

func lessonBuilding(room string) string {
	parts := explicitBuilding.FindStringSubmatch(room)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

type LessonChange struct {
	Before   *domain.Lesson
	After    *domain.Lesson
	Fields   []string
	Priority int
}

func DetailedLessonChanges(before, after []domain.Lesson) []LessonChange {
	diff := CompareLessonSnapshots(before, after)
	old, next := diff.RemovedItems, diff.AddedItems
	compatible := func(a, b domain.Lesson) bool {
		if a.GroupID != b.GroupID || a.Subgroup != b.Subgroup {
			return false
		}
		if a.ID != "" && a.ID == b.ID {
			return true
		}
		return searchtext.TokenKey(a.Subject) == searchtext.TokenKey(b.Subject) && a.Type == b.Type
	}
	used := map[int]bool{}
	result := []LessonChange{}
	first := ""
	for _, l := range before {
		if first == "" || l.TimeStart < first {
			first = l.TimeStart
		}
	}
	for i := range old {
		candidates := []int{}
		for j := range next {
			if compatible(old[i], next[j]) {
				candidates = append(candidates, j)
			}
		}
		match := -1
		if len(candidates) == 1 {
			j := candidates[0]
			reverse := 0
			for _, item := range old {
				if compatible(item, next[j]) {
					reverse++
				}
			}
			if reverse == 1 && !used[j] {
				match = j
			}
		}
		change := LessonChange{Before: &old[i], Priority: 40}
		if match < 0 {
			if old[i].TimeStart == first {
				change.Priority = 100
			}
			result = append(result, change)
			continue
		}
		used[match] = true
		change.After = &next[match]
		a, b := old[i], next[match]
		if a.TimeStart != b.TimeStart || a.TimeEnd != b.TimeEnd {
			change.Fields = append(change.Fields, fmt.Sprintf("время %s–%s → %s–%s", a.TimeStart, a.TimeEnd, b.TimeStart, b.TimeEnd))
			change.Priority = 90
		}
		if a.Room != b.Room {
			change.Fields = append(change.Fields, "аудитория "+displayChangeValue(a.Room)+" → "+displayChangeValue(b.Room))
			change.Priority = max(change.Priority, 70)
			if building := lessonBuilding(a.Room); building != "" && lessonBuilding(b.Room) != "" && building != lessonBuilding(b.Room) {
				change.Priority = max(change.Priority, 80)
			}
		}
		if a.Teacher != b.Teacher {
			change.Fields = append(change.Fields, "преподаватель "+displayChangeValue(a.Teacher)+" → "+displayChangeValue(b.Teacher))
		}
		if a.Subject != b.Subject {
			change.Fields = append(change.Fields, "предмет "+a.Subject+" → "+b.Subject)
		}
		if a.Type != b.Type {
			change.Fields = append(change.Fields, "тип занятия "+string(a.Type)+" → "+string(b.Type))
		}
		if len(change.Fields) > 0 {
			result = append(result, change)
		}
	}
	for j := range next {
		if !used[j] {
			result = append(result, LessonChange{After: &next[j], Priority: 30})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Priority > result[j].Priority })
	return result
}

func displayChangeValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "не указано"
	}
	return value
}

func transitionDay(lessons []domain.Lesson, semesters []domain.Semester, target domain.PersonalTarget, date time.Time, changes []domain.PersonalOverride) ([]domain.Lesson, error) {
	result := []domain.Lesson{}
	for _, lesson := range lessons {
		if target.Role == domain.RoleStudent && (lesson.GroupID != target.ID || target.Subgroup > 0 && lesson.Subgroup > 0 && lesson.Subgroup != target.Subgroup) {
			continue
		}
		if target.Role == domain.RoleTeacher {
			matches := false
			for _, name := range searchtext.TeacherNames([]string{lesson.Teacher}) {
				if searchtext.TokenKey(name) == searchtext.TokenKey(target.Name) {
					matches = true
				}
			}
			if !matches {
				continue
			}
		}
		var start *time.Time
		inSemester := true
		for _, semester := range semesters {
			if semester.ID == lesson.SemesterID {
				start = &semester.StartDate
				inSemester = !date.Before(helpers.NormalizeDate(semester.StartDate)) && !date.After(helpers.NormalizeDate(semester.EndDate))
				break
			}
		}
		if !inSemester || !LessonMatchesDate(lesson, date, start) {
			continue
		}
		item, err := applyPersonalLesson(lesson, date, changes)
		if err != nil {
			return nil, err
		}
		if item.Cancelled {
			continue
		}
		l := item.Lesson
		l.WeekType = domain.WeekTypeDate
		l.DayOfWeek = helpers.Weekday(date)
		l.SpecialDate = &date
		l.ValidFrom = nil
		l.ValidTo = nil
		l.Recurrence = domain.RecurrenceRule{}
		result = append(result, l)
	}
	sortLessons(result)
	return result, nil
}

func DescribeScheduleTransition(transition domain.ScheduleTransition, target domain.PersonalTarget, changes []domain.PersonalOverride, now time.Time) (string, []string, error) {
	location, err := time.LoadLocation(target.Timezone)
	if err != nil {
		return "", nil, err
	}
	from := helpers.NormalizeDate(now.In(location))
	until := from.AddDate(0, 0, 30)
	for _, semester := range append(append([]domain.Semester(nil), transition.Semesters...), transition.BeforeSemesters...) {
		if semester.EndDate.After(until) {
			until = helpers.NormalizeDate(semester.EndDate)
		}
	}
	if until.After(from.AddDate(1, 0, 0)) {
		until = from.AddDate(1, 0, 0)
	}
	beforeSemesters := transition.BeforeSemesters
	if beforeSemesters == nil {
		beforeSemesters = transition.Semesters
	}
	lines := []string{}
	dates := []string{}
	total := 0
	shown := 0
	for date := from; !date.After(until); date = date.AddDate(0, 0, 1) {
		before, err := transitionDay(transition.Before, beforeSemesters, target, date, changes)
		if err != nil {
			return "", nil, err
		}
		after, err := transitionDay(transition.After, transition.Semesters, target, date, changes)
		if err != nil {
			return "", nil, err
		}
		diff := DetailedLessonChanges(before, after)
		if len(diff) == 0 {
			continue
		}
		total += len(diff)
		if len(dates) < 3 {
			dates = append(dates, date.Format("20060102"))
		}
		if shown >= 6 {
			continue
		}
		label := date.Format("02.01")
		if date.Equal(from) {
			label = "Сегодня"
		} else if date.Equal(from.AddDate(0, 0, 1)) {
			label = "Завтра"
		}
		lines = append(lines, label+":")
		for _, change := range diff {
			if shown >= 6 {
				break
			}
			shown++
			if change.Before == nil {
				l := change.After
				lines = append(lines, fmt.Sprintf("+ %s: добавлено %s–%s, %s", changeSubject(*l, target), l.TimeStart, l.TimeEnd, displayChangeValue(l.Room)))
			} else if change.After == nil {
				l := change.Before
				lines = append(lines, fmt.Sprintf("− %s: убрано из расписания (%s–%s)", changeSubject(*l, target), l.TimeStart, l.TimeEnd))
			} else {
				lines = append(lines, changeSubject(*change.After, target)+": "+strings.Join(change.Fields, "; ")+".")
			}
		}
		if len(before) > 0 && len(after) > 0 && before[0].TimeStart != after[0].TimeStart {
			lines = append(lines, "Первая пара теперь в "+after[0].TimeStart+".")
		} else if len(before) > 0 && len(after) == 0 {
			lines = append(lines, "В этот день занятий больше нет.")
		}
	}
	if total > shown {
		lines = append(lines, fmt.Sprintf("Ещё изменений в следующих занятиях: %d. Откройте расписание для подробностей.", total-shown))
	}
	for _, change := range changes {
		if change.NeedsReview {
			lines = append(lines, "Ваши личные правки приостановлены. Откройте личное расписание, чтобы согласовать их после обновления вуза.")
			break
		}
	}
	return strings.Join(lines, "\n"), dates, nil
}

func (s *ScheduleService) DescribeNotifications(ctx context.Context, userID string, items []domain.NotificationDelivery) ([]domain.NotificationDelivery, error) {
	targets, err := s.personalRepo.NotificationTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	changes, err := s.personalRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := append([]domain.NotificationDelivery(nil), items...)
	groups := map[string][]int{}
	for i, item := range items {
		if len(item.ChangeContext) > 2 {
			groups[item.GroupID] = append(groups[item.GroupID], i)
		}
	}
	for groupID, indexes := range groups {
		var target *domain.PersonalTarget
		for i := range targets {
			if targets[i].ID == groupID || groupID == "" && targets[i].Role == domain.RoleTeacher {
				target = &targets[i]
				break
			}
		}
		if target == nil {
			for _, i := range indexes {
				result[i].Summary = ""
			}
			continue
		}
		var merged domain.ScheduleTransition
		for n, i := range indexes {
			var transition domain.ScheduleTransition
			if err = json.Unmarshal(items[i].ChangeContext, &transition); err != nil {
				return nil, err
			}
			if n == 0 {
				merged = transition
			} else {
				merged.After = transition.After
				merged.Semesters = transition.Semesters
			}
			result[i].Summary = ""
			result[i].Dates = nil
		}
		applicable := []domain.PersonalOverride{}
		for _, change := range changes {
			if change.Role == target.Role && change.TargetID == target.ID {
				applicable = append(applicable, change)
			}
		}
		summary, dates, err := DescribeScheduleTransition(merged, *target, applicable, time.Now())
		if err != nil {
			return nil, err
		}
		first := indexes[0]
		result[first].Summary = summary
		result[first].Dates = dates
		result[first].TargetToken = keyboards.GroupToken(target.ID)
		if target.Role == domain.RoleTeacher {
			result[first].TargetToken = keyboards.TeacherToken(target.UniversityID, target.Name)
		}
	}
	ordered := make([]domain.NotificationDelivery, 0, len(result))
	added := map[int]bool{}
	for i := range result {
		if added[i] {
			continue
		}
		if len(result[i].ChangeContext) <= 2 {
			ordered = append(ordered, result[i])
			added[i] = true
			continue
		}
		for _, j := range groups[result[i].GroupID] {
			ordered = append(ordered, result[j])
			added[j] = true
		}
	}
	return ordered, nil
}

func changeSubject(lesson domain.Lesson, target domain.PersonalTarget) string {
	if target.Role == domain.RoleTeacher && lesson.GroupName != "" {
		return lesson.Subject + " · " + lesson.GroupName
	}
	return lesson.Subject
}
