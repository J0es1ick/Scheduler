package domain

type ReminderRecipient struct {
	UserID          string `db:"user_id"`
	GroupID         string `db:"group_id"`
	GroupName       string `db:"group_name"`
	UniversityName  string `db:"university_name"`
	ReminderMinutes int    `db:"reminder_minutes"`
}
