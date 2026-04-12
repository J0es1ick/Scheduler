package domain

import "time"

type Group struct {
	ID           string    `db:"id"`
	UniversityID string    `db:"university_id"`
	Name         string    `db:"name"`
	IsActive     bool      `db:"is_active"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}