package domain

import "time"

type ScheduleEntry struct {
	ID       string    `db:"id"`
	LessonID string    `db:"lesson_id"`
	Date     time.Time `db:"date"`
}

// LessonOccurrence — конкретная пара урок + дата, нужна для графика без шаблонов.
type LessonOccurrence struct {
	Lesson Lesson    `json:"lesson"`
	Date   time.Time `json:"date"`
}
