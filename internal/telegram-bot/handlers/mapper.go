package handlers

import (
	"sort"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func mapToDaySchedule(data map[time.Time][]domain.Lesson) []dto.DaySchedule {
	var result []dto.DaySchedule

	for date, lessons := range data {
		if len(lessons) == 0 {
			continue
		}

		result = append(result, dto.DaySchedule{
			Date:    date,
			Lessons: lessons,
		})
	}

	// ВАЖНО: сортировка по дате
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})

	return result
}
