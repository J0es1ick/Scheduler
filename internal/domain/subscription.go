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