package domain

import "time"

const LessonReminderWorker = "lesson_reminders"

type WorkerStatus struct {
	Name            string     `json:"name" db:"name"`
	Cursor          string     `json:"cursor" db:"cursor"`
	LastStartedAt   *time.Time `json:"last_started_at" db:"last_started_at"`
	LastFinishedAt  *time.Time `json:"last_finished_at" db:"last_finished_at"`
	LastFullCycleAt *time.Time `json:"last_full_cycle_at" db:"last_full_cycle_at"`
	LastDurationMS  int64      `json:"last_duration_ms" db:"last_duration_ms"`
	LastProcessed   int        `json:"last_processed" db:"last_processed"`
	LastFailures    int        `json:"last_failures" db:"last_failures"`
	LastError       string     `json:"last_error" db:"last_error"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type WorkerRunResult struct {
	Name            string
	Cursor          string
	StartedAt       time.Time
	FinishedAt      time.Time
	LastFullCycleAt *time.Time
	Processed       int
	Failures        int
	LastError       string
}
