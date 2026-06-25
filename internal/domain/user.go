package domain

import "time"

type User struct {
	ID           string    `db:"id"`
	Username     string    `db:"username"`
	IsAdmin      bool      `db:"is_admin"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}