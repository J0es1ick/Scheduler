package domain

import "time"

type ScheduleViewFormat string

const (
	ScheduleViewCompact ScheduleViewFormat = "compact"
	ScheduleViewVisual  ScheduleViewFormat = "visual"
)

type Subscription struct {
	ID                 string             `db:"id" json:"id"`
	UserID             string             `db:"user_id" json:"user_id"`
	ObjectID           string             `db:"object_id" json:"object_id"`
	ObjectType         string             `db:"object_type" json:"object_type"`
	ScheduleViewFormat ScheduleViewFormat `db:"schedule_view_format" json:"schedule_view_format"`
	Subgroup           int                `db:"subgroup" json:"subgroup"`
	CreatedAt          time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time          `db:"updated_at" json:"updated_at"`
}

type GroupSubscription struct {
	ID                 string             `db:"id"`
	UserID             string             `db:"user_id"`
	GroupID            string             `db:"group_id"`
	GroupName          string             `db:"group_name"`
	UniversityID       string             `db:"university_id"`
	UniversityName     string             `db:"university_name"`
	IsDefault          bool               `db:"is_default"`
	IsActive           bool               `db:"is_active"`
	ScheduleViewFormat ScheduleViewFormat `db:"schedule_view_format"`
	Subgroup           int                `db:"subgroup"`
	CreatedAt          time.Time          `db:"created_at"`
	UpdatedAt          time.Time          `db:"updated_at"`
}
