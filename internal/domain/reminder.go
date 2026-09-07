package domain

import "time"

type ReminderRecipient struct {
	UserID          string `db:"user_id"`
	GroupID         string `db:"group_id"`
	GroupName       string `db:"group_name"`
	UniversityName  string `db:"university_name"`
	Timezone        string `db:"timezone"`
	ReminderMinutes int    `db:"reminder_minutes"`
	Subgroup        int    `db:"subgroup"`
}

type ReminderContext struct {
	Date      string    `json:"date"`
	TimeStart string    `json:"time_start"`
	TimeEnd   string    `json:"time_end"`
	Subgroup  int       `json:"subgroup"`
	StartsAt  time.Time `json:"starts_at"`
}
