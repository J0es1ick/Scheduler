package domain

import "time"

type LessonType string
type WeekType string

const (
	LessonTypeLecture      LessonType = "lecture"
	LessonTypePractice     LessonType = "practice"
	LessonTypeLab          LessonType = "lab"
	LessonTypeSeminar      LessonType = "seminar"
	LessonTypeExam         LessonType = "exam"
	LessonTypeCredit       LessonType = "credit"
	LessonTypeConsultation LessonType = "consultation"
)

const (
	WeekTypeEvery WeekType = "every" // каждую неделю
	WeekTypeOdd   WeekType = "odd"   // нечётная неделя
	WeekTypeEven  WeekType = "even"  // чётная неделя
	WeekTypeDate  WeekType = "date"  // точная дата (одноразовое расписание)
)

type Lesson struct {
	ID           string     `db:"id"`
	UniversityID string     `db:"university_id"`
	SemesterID   string     `db:"semester_id"`  // к какому семестру относится
	DayOfWeek    int        `db:"day_of_week"`  // 1=пн, 2=вт, ..., 6=сб
	SpecialDate  *time.Time `db:"special_date"` // для одноразового расписания (week_type = date) Это поле может быть NULL, если урок повторяющийся, возможно будет мозгоёбкой в будущем
	TimeStart    string     `db:"time_start"`   // "09:00" — только время
	TimeEnd      string     `db:"time_end"`     // "10:30"
	WeekType     WeekType   `db:"week_type"`    // every / odd / even
	Subject      string     `db:"subject"`
	Type         LessonType `db:"type"`
	Teacher      string     `db:"teacher"`
	Room         string     `db:"room"`
	GroupID      string     `db:"group_id"`
	Subgroup     int        `db:"subgroup"`
	ValidFrom    *time.Time `db:"valid_from"` // первая дата действия занятия на сайте источника
	ValidTo      *time.Time `db:"valid_to"`   // последняя дата действия занятия на сайте источника
	UpdatedAt    time.Time  `db:"updated_at"`
}
