package domain

import "time"

type Semester struct {
	ID           string    `db:"id"`
	UniversityID string    `db:"university_id"`
	ExternalID   string    `db:"external_id"`
	Name         string    `db:"name"`
	AcademicYear string    `db:"academic_year"`
	Status       string    `db:"status"`
	StartDate    time.Time `db:"start_date"`
	EndDate      time.Time `db:"end_date"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
