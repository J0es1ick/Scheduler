package domain

import "time"

type Semester struct {
	ID           string    `db:"id"`
	UniversityID string    `db:"university_id"`
	Name         string    `db:"name"`
	StartDate    time.Time `db:"start_date"`
	EndDate      time.Time `db:"end_date"`
}