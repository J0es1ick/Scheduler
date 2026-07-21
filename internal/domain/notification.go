package domain

import "time"

type NotificationDelivery struct {
	ID             string     `db:"id"`
	EventID        string     `db:"event_id"`
	UserID         string     `db:"user_id"`
	GroupID        string     `db:"group_id"`
	GroupName      string     `db:"group_name"`
	UniversityName string     `db:"university_name"`
	Source         string     `db:"source"`
	Summary        string     `db:"summary"`
	Attempts       int        `db:"attempts"`
	NextAttemptAt  time.Time  `db:"next_attempt_at"`
	CreatedAt      time.Time  `db:"created_at"`
	DeliveredAt    *time.Time `db:"delivered_at"`
}
