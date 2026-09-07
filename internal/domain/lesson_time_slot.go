package domain

type LessonTimeSlot struct {
	Start string `db:"time_start"`
	End   string `db:"time_end"`
}
