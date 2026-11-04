package service

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
)

type PersonalAgenda struct {
	Target    domain.PersonalTarget   `json:"target"`
	Days      []PersonalDay           `json:"days"`
	Freshness *domain.SourceFreshness `json:"freshness"`
}

func (s *ScheduleService) PersonalAgenda(ctx context.Context, userID, targetID string, from, to time.Time) (*PersonalAgenda, error) {
	targets, err := s.PersonalTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	var target *domain.PersonalTarget
	for i := range targets {
		if targets[i].ID == targetID {
			target = &targets[i]
			break
		}
	}
	if target == nil {
		return nil, sql.ErrNoRows
	}
	lessons, err := s.targetLessons(ctx, *target)
	if err != nil {
		return nil, err
	}
	data, err := s.lessonsForRange(ctx, lessons, from, to)
	if err != nil {
		return nil, err
	}
	changes, err := s.personalRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	applicable := []domain.PersonalOverride{}
	for _, change := range changes {
		if change.Role == target.Role && change.TargetID == target.ID {
			applicable = append(applicable, change)
		}
	}
	freshness, err := s.personalRepo.Freshness(ctx, target.UniversityID)
	if err != nil {
		return nil, err
	}
	result := &PersonalAgenda{Target: *target, Freshness: freshness, Days: []PersonalDay{}}
	for date := helpers.NormalizeDate(from); !date.After(helpers.NormalizeDate(to)); date = date.AddDate(0, 0, 1) {
		day := PersonalDay{Date: date.Format(time.DateOnly), Lessons: []PersonalLesson{}}
		for _, lesson := range data[date] {
			if target.Subgroup > 0 && lesson.Subgroup > 0 && lesson.Subgroup != target.Subgroup {
				continue
			}
			item, err := applyPersonalLesson(lesson, date, applicable)
			if err != nil {
				return nil, err
			}
			item.GroupName = lesson.GroupName
			day.Lessons = append(day.Lessons, item)
		}
		sort.SliceStable(day.Lessons, func(i, j int) bool { return day.Lessons[i].Lesson.TimeStart < day.Lessons[j].Lesson.TimeStart })
		result.Days = append(result.Days, day)
	}
	return result, nil
}

type PlannedNotification struct {
	Kind  string    `json:"kind"`
	At    time.Time `json:"at"`
	Date  string    `json:"date"`
	Label string    `json:"label"`
}

func PreviewNotifications(p domain.NotificationPreferences, agenda *PersonalAgenda, now time.Time) []PlannedNotification {
	result := []PlannedNotification{}
	if agenda == nil {
		return result
	}
	zone, err := time.LoadLocation(agenda.Target.Timezone)
	if err != nil {
		return result
	}
	local := now.In(zone)
	byDate := map[string][]domain.Lesson{}
	for _, day := range agenda.Days {
		for _, item := range day.Lessons {
			if !item.Cancelled {
				byDate[day.Date] = append(byDate[day.Date], item.Lesson)
			}
		}
	}
	for i := 0; i < 7; i++ {
		day := time.Date(local.Year(), local.Month(), local.Day()+i, 0, 0, 0, 0, zone)
		date := day.Format(time.DateOnly)
		if p.PausedUntil != "" && date < p.PausedUntil {
			continue
		}
		at, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+p.DailyTime, zone)
		scheduleDate := day.AddDate(0, 0, p.DailyDayOffset).Format(time.DateOnly)
		published := agenda.Freshness != nil && agenda.Freshness.LastSuccess != nil
		if p.DailyOn(day) && at.After(now) && !(p.DailySkipEmpty && published && len(byDate[scheduleDate]) == 0) {
			label := "Расписание на сегодня"
			if p.DailyDayOffset == 1 {
				label = "Расписание на завтра"
			}
			result = append(result, PlannedNotification{Kind: "daily", At: at, Date: scheduleDate, Label: label})
		}
		seen := map[string]bool{}
		for _, lesson := range byDate[date] {
			if !p.ReminderEnabled || seen[lesson.TimeStart] {
				continue
			}
			seen[lesson.TimeStart] = true
			start, err := time.ParseInLocation("2006-01-02 15:04", date+" "+lesson.TimeStart, zone)
			if err != nil {
				continue
			}
			at := start.Add(-time.Duration(p.ReminderMinutes) * time.Minute)
			if at.After(now) && !p.InQuietHours(at) && (p.PausedUntil == "" || at.Format(time.DateOnly) >= p.PausedUntil) {
				result = append(result, PlannedNotification{Kind: "reminder", At: at, Date: date, Label: lesson.Subject})
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].At.Before(result[j].At) })
	if len(result) > 12 {
		result = result[:12]
	}
	return result
}
