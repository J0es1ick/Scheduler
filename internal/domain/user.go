package domain

import "time"

type User struct {
	ID                   string    `db:"id" json:"id"`
	Username             string    `db:"username" json:"username"`
	IsAdmin              bool      `db:"is_admin" json:"is_admin"`
	DefaultGroupID       string    `db:"default_group_id" json:"default_group_id"`
	NotificationsEnabled bool      `db:"notifications_enabled" json:"notifications_enabled"`
	ReminderEnabled      bool      `db:"reminder_enabled" json:"reminder_enabled"`
	ReminderMinutes      int       `db:"reminder_minutes" json:"reminder_minutes"`
	QuietHoursEnabled    bool      `db:"quiet_hours_enabled" json:"quiet_hours_enabled"`
	QuietHoursStart      string    `db:"quiet_hours_start" json:"quiet_hours_start"`
	QuietHoursEnd        string    `db:"quiet_hours_end" json:"quiet_hours_end"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at" json:"updated_at"`
}

type UserDataExport struct {
	ExportedAt      time.Time        `json:"exported_at"`
	User            User             `json:"user"`
	Subscriptions   []Subscription   `json:"subscriptions"`
	SupportRequests []SupportRequest `json:"support_requests"`
}
