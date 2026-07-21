package domain

import "time"

type Subscription struct {
	ID         string    `db:"id"`
	UserID     string    `db:"user_id"`
	ObjectID   string    `db:"object_id"`   // ID группы или семестра
	ObjectType string    `db:"object_type"` // "group", "teacher", "room"
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type GroupSubscription struct {
	ID             string    `db:"id"`
	UserID         string    `db:"user_id"`
	GroupID        string    `db:"group_id"`
	GroupName      string    `db:"group_name"`
	UniversityID   string    `db:"university_id"`
	UniversityName string    `db:"university_name"`
	IsDefault      bool      `db:"is_default"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
