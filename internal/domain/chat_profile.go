package domain

import "time"

type ChatScheduleProfile struct {
	ChatID         string             `db:"chat_id"`
	Title          string             `db:"title"`
	DefaultGroupID string             `db:"default_group_id"`
	GroupName      string             `db:"group_name"`
	UniversityID   string             `db:"university_id"`
	UniversityName string             `db:"university_name"`
	ViewFormat     ScheduleViewFormat `db:"schedule_view_format"`
	ConfiguredBy   string             `db:"configured_by"`
	CreatedAt      time.Time          `db:"created_at"`
	UpdatedAt      time.Time          `db:"updated_at"`
}
