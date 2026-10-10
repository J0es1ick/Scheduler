package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
)

type SchedulePattern struct {
	Kind        string `json:"kind"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Weeks       int    `json:"weeks"`
	Period      int    `json:"period"`
	Exceptions  int    `json:"exceptions"`
	occurrences map[string][]domain.PersonalOccurrence
}

func patternLessonKey(l domain.Lesson) string {
	if l.SpecialDate != nil {
		l.DayOfWeek = helpers.Weekday(*l.SpecialDate)
	}
	normalize := func(s string) string {
		return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "ё", "е"))), " ")
	}
	v, _ := json.Marshal([]any{l.GroupID, l.SemesterID, l.DayOfWeek, l.TimeStart, l.TimeEnd, l.Subgroup, normalize(l.Subject), l.Type, normalize(l.Teacher), normalize(l.Room)})
	return string(v)
}

func monday(date time.Time) time.Time {
	date = helpers.NormalizeDate(date)
	return date.AddDate(0, 0, 1-helpers.Weekday(date))
}

func AnalyzeSchedulePattern(lessons []domain.Lesson, semester domain.Semester) SchedulePattern {
	result := SchedulePattern{Kind: "individual", occurrences: map[string][]domain.PersonalOccurrence{}}
	start, end := helpers.NormalizeDate(semester.StartDate), helpers.NormalizeDate(semester.EndDate)
	if end.Before(start) || end.Sub(start) > 370*24*time.Hour {
		return result
	}
	first := monday(start)
	weeks := int(monday(end).Sub(first).Hours()/168) + 1
	signatures := make([][]string, weeks)
	type occurrence struct {
		key   string
		value domain.PersonalOccurrence
		week  int
	}
	var all []occurrence
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		week := int(monday(date).Sub(first).Hours() / 168)
		for _, lesson := range lessons {
			if lesson.SemesterID != semester.ID || !LessonMatchesDate(lesson, date, &semester.StartDate) {
				continue
			}
			key := patternLessonKey(lesson)
			signatures[week] = append(signatures[week], key)
			id := lesson.PersonalKey
			if id == "" {
				id = lesson.ID
			}
			all = append(all, occurrence{key, domain.PersonalOccurrence{LessonID: id, Date: date.Format(time.DateOnly)}, week})
		}
	}
	encoded := make([]string, weeks)
	for i := range signatures {
		sort.Strings(signatures[i])
		encoded[i] = strings.Join(signatures[i], "\n")
	}
	firstActive, lastActive := weeks, -1
	for i, value := range encoded {
		if value != "" {
			firstActive = min(firstActive, i)
			lastActive = max(lastActive, i)
		}
	}
	var templates []string
	globalPeriod := 0
	globalMatches := 0
	for _, period := range []int{1, 2} {
		modes := make([]string, period)
		counts := make([]int, period)
		for phase := 0; phase < period; phase++ {
			frequencies := map[string]int{}
			for week := firstActive; week <= lastActive; week++ {
				if week%period != phase {
					continue
				}
				value := encoded[week]
				frequencies[value]++
				if frequencies[value] > counts[phase] {
					modes[phase] = value
					counts[phase] = frequencies[value]
				}
			}
		}
		minimum := 4
		if period == 2 {
			minimum = 3
		}
		valid, matches, nonempty := true, 0, false
		for phase, count := range counts {
			valid = valid && count >= minimum
			matches += count
			nonempty = nonempty || modes[phase] != ""
		}
		if valid && nonempty && matches*10 >= (lastActive-firstActive+1)*8 {
			globalPeriod, globalMatches, templates = period, matches, modes
			break
		}
	}
	bestStart, bestEnd, bestPeriod := 0, -1, 0
	for _, period := range []int{1, 2} {
		minimum := 4
		if period == 2 {
			minimum = 6
		}
		for left := 0; left+minimum <= weeks; left++ {
			right := left + period
			for right < weeks && encoded[right] == encoded[right-period] {
				right++
			}
			if right-left < minimum || (encoded[left] == "" && (period == 1 || encoded[left+1] == "")) {
				continue
			}
			if right-left > bestEnd-bestStart+1 {
				bestStart, bestEnd, bestPeriod = left, right-1, period
			}
		}
	}
	if globalPeriod > 0 {
		bestStart, bestEnd, bestPeriod = firstActive, lastActive, globalPeriod
	}
	if bestPeriod == 0 {
		return result
	}
	subset := func(a, b []string) bool {
		counts := map[string]int{}
		for _, v := range b {
			counts[v]++
		}
		for _, v := range a {
			counts[v]--
			if counts[v] < 0 {
				return false
			}
		}
		return true
	}
	if bestStart == 1 && start.After(first) && subset(signatures[0], signatures[bestPeriod]) {
		bestStart = 0
	}
	if bestEnd == weeks-2 && end.Before(monday(end).AddDate(0, 0, 6)) && subset(signatures[weeks-1], signatures[weeks-1-bestPeriod]) {
		bestEnd++
	}
	result.Kind = "weekly"
	if bestPeriod == 2 {
		result.Kind = "biweekly"
	}
	if (bestEnd-bestStart+1)*10 < (lastActive-firstActive+1)*7 {
		result.Kind = "individual"
	}
	result.Period = bestPeriod
	if globalPeriod > 0 {
		result.Exceptions = lastActive - firstActive + 1 - globalMatches
	}
	result.Weeks = bestEnd - bestStart + 1
	result.From = max(start.Format(time.DateOnly), first.AddDate(0, 0, bestStart*7).Format(time.DateOnly))
	result.To = min(end.Format(time.DateOnly), first.AddDate(0, 0, bestEnd*7+6).Format(time.DateOnly))
	for _, item := range all {
		if item.week >= bestStart && item.week <= bestEnd {
			if globalPeriod > 0 {
				found := false
				for _, key := range strings.Split(templates[item.week%globalPeriod], "\n") {
					if key == item.key {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			result.occurrences[item.key] = append(result.occurrences[item.key], item.value)
		}
	}
	return result
}

func (p SchedulePattern) Repeats(lesson domain.Lesson, date time.Time) []domain.PersonalOccurrence {
	day := date.Format(time.DateOnly)
	if day < p.From || day > p.To {
		return nil
	}
	result := []domain.PersonalOccurrence{}
	seen := map[string]bool{}
	for _, item := range p.occurrences[patternLessonKey(lesson)] {
		if seen[item.Date] {
			return nil
		}
		seen[item.Date] = true
		if item.Date >= day {
			result = append(result, item)
		}
	}
	if len(result) < 2 {
		return nil
	}
	return result
}
