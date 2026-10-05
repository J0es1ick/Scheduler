package domain

type UserRole string

const (
	RoleStudent UserRole = "student"
	RoleTeacher UserRole = "teacher"
)

type Teacher struct {
	ID           string `db:"id" json:"id"`
	UniversityID string `db:"university_id" json:"university_id"`
	Name         string `db:"name" json:"name"`
	NameKey      string `db:"name_key" json:"name_key"`
}
