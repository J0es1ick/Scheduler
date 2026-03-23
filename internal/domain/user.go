package domain

import "time"

type User struct {
	ID           string    `db:"id"`
	Username     string    `db:"username"`
	IsAdmin      bool      `db:"is_admin"`
	UniversityID string    `db:"university_id"`
	Group        string    `db:"group"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}