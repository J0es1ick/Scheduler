package domain

import (
	"encoding/json"
	"time"
)

type PersonalTarget struct {
	ID             string   `db:"id" json:"id"`
	Role           UserRole `db:"role" json:"role"`
	Name           string   `db:"name" json:"name"`
	UniversityID   string   `db:"university_id" json:"university_id"`
	UniversityName string   `db:"university_name" json:"university_name"`
	Timezone       string   `db:"timezone" json:"timezone"`
	Subgroup       int      `db:"subgroup" json:"subgroup"`
	Primary        bool     `db:"is_primary" json:"primary"`
}

type PersonalLessonPatch struct {
	Subject   *string     `json:"subject,omitempty"`
	Teacher   *string     `json:"teacher,omitempty"`
	Room      *string     `json:"room,omitempty"`
	Type      *LessonType `json:"type,omitempty"`
	TimeStart *string     `json:"time_start,omitempty"`
	TimeEnd   *string     `json:"time_end,omitempty"`
}

func (p PersonalLessonPatch) Apply(lesson Lesson) Lesson {
	if p.Subject != nil {
		lesson.Subject = *p.Subject
	}
	if p.Teacher != nil {
		lesson.Teacher = *p.Teacher
	}
	if p.Room != nil {
		lesson.Room = *p.Room
	}
	if p.Type != nil {
		lesson.Type = *p.Type
	}
	if p.TimeStart != nil {
		lesson.TimeStart = *p.TimeStart
	}
	if p.TimeEnd != nil {
		lesson.TimeEnd = *p.TimeEnd
	}
	return lesson
}

type PersonalOverride struct {
	ID           string          `db:"id" json:"id"`
	UserID       string          `db:"user_id" json:"-"`
	Role         UserRole        `db:"role" json:"role"`
	TargetID     string          `db:"target_id" json:"target_id"`
	LessonID     string          `db:"lesson_id" json:"lesson_id"`
	UniversityID string          `db:"university_id" json:"university_id"`
	SemesterID   string          `db:"semester_id" json:"semester_id"`
	Scope        string          `db:"scope" json:"scope"`
	ValidFrom    time.Time       `db:"valid_from" json:"valid_from"`
	ValidTo      time.Time       `db:"valid_to" json:"valid_to"`
	Patch        json.RawMessage `db:"patch" json:"patch"`
	Cancelled    bool            `db:"cancelled" json:"cancelled"`
	Version      int64           `db:"version" json:"version"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updated_at"`
}

func (o PersonalOverride) Matches(lesson Lesson, date time.Time) bool {
	key := lesson.PersonalKey
	if key == "" {
		key = lesson.ID
	}
	day := date.Format(time.DateOnly)
	return o.LessonID == key && o.SemesterID == lesson.SemesterID && day >= o.ValidFrom.Format(time.DateOnly) && day <= o.ValidTo.Format(time.DateOnly)
}
