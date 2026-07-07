package helpers

import (
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func NormalizeDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func Weekday(t time.Time) int {
	d := int(t.Weekday())
	if d == 0 {
		return 7
	}
	return d
}

func DetermineWeekType(date, semesterStart time.Time) domain.WeekType {
	daysDiff := int(date.Sub(semesterStart).Hours() / 24)
	if daysDiff < 0 {
		daysDiff = 0
	}
	weekNumber := daysDiff/7 + 1
	if weekNumber%2 == 1 {
		return domain.WeekTypeOdd
	}
	return domain.WeekTypeEven
}

func MatchesWeekType(lessonWeekType, currentWeekType domain.WeekType) bool {
	if lessonWeekType == domain.WeekTypeEvery {
		return true
	}
	return lessonWeekType == currentWeekType
}