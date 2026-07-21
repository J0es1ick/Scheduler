package domain

import "time"

type User struct {
	ID                   string    `db:"id"`
	Username             string    `db:"username"`
	IsAdmin              bool      `db:"is_admin"`
	DefaultGroupID       string    `db:"default_group_id"`
	NotificationsEnabled bool      `db:"notifications_enabled"`
	CreatedAt            time.Time `db:"created_at"`
	UpdatedAt            time.Time `db:"updated_at"`
}
