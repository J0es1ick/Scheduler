package domain

import "time"

type LessonType string

const (
    LessonTypeLecture  LessonType = "lecture"
    LessonTypePractice LessonType = "practice"
    LessonTypeLab      LessonType = "lab"
    LessonTypeSeminar  LessonType = "seminar"
)

type Lesson struct {
    ID           string     `db:"id"`            // первичный ключ отсутствует
    UniversityID string     `db:"university_id"` // к какому университету относится пара
    TimeStart    time.Time  `db:"time_start"`
    TimeEnd      time.Time  `db:"time_end"`
    Subject      string     `db:"subject"`
    Type         LessonType `db:"type"`
    Teacher      string     `db:"teacher"`
    Room         string     `db:"room"`
    Group        string     `db:"group"`
    Subgroup     int        `db:"subgroup"`      // 0 = вся группа, 1/2 = подгруппа (актуально для лаб)
    WeekNumber   int        `db:"week_number"`
    IsEvenWeek   bool       `db:"is_even_week"`  // чётная/нечётная — часто нужно отдельно
}
