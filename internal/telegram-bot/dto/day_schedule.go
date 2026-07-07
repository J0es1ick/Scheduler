package dto

import (
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

// DaySchedule группирует занятия по одной дате.
// Используется только в слое бота для отображения — в БД не хранится.
// Собирается из []Lesson через lessonMapToDaySchedules в handlers.
type DaySchedule struct {
	Date    time.Time
	Lessons []domain.Lesson
}
