package service

import (
	"sort"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
)

func TeacherChanges(before, after []domain.Lesson) map[string]string {
	group := func(lessons []domain.Lesson) map[string][]domain.Lesson {
		result := make(map[string][]domain.Lesson)
		for _, lesson := range lessons {
			names := searchtext.TeacherNames([]string{lesson.Teacher})
			keys := make([]string, len(names))
			for i, name := range names {
				keys[i] = searchtext.TokenKey(name)
			}
			sort.Strings(keys)
			lesson.Teacher = strings.Join(keys, ";")
			for _, key := range keys {
				result[key] = append(result[key], lesson)
			}
		}
		return result
	}
	old, current := group(before), group(after)
	keys := make(map[string]bool)
	for key := range old {
		keys[key] = true
	}
	for key := range current {
		keys[key] = true
	}
	result := make(map[string]string)
	for key := range keys {
		diff := CompareLessonSnapshots(old[key], current[key])
		if diff.Changed() {
			result[key] = scheduleChangeSummary(diff)
		}
	}
	return result
}
