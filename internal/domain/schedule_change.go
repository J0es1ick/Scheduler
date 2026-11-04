package domain

type ScheduleTransition struct {
	UniversityID    string     `json:"university_id"`
	Before          []Lesson   `json:"before"`
	After           []Lesson   `json:"after"`
	Semesters       []Semester `json:"semesters"`
	BeforeSemesters []Semester `json:"before_semesters,omitempty"`
}
