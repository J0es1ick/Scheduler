package domain

import "time"

type DaySchedule struct {
	Date    time.Time `db:"date"`
	Lessons []Lesson  `db:"lessons"`
}