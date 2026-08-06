package domain

import "time"

type DataSource struct {
	ID                  string     `db:"id"`
	UniversityID        string     `db:"university_id"`
	AdapterType         string     `db:"adapter_type"`
	Config              string     `db:"config"`
	IsEnabled           bool       `db:"is_enabled"`
	UpdateInterval      int        `db:"update_interval"` // В секундах
	LastRunAt           time.Time  `db:"last_run_at"`
	LastSuccessAt       *time.Time `db:"last_success_at"`
	LastError           string     `db:"last_error"`
	ConsecutiveFailures int        `db:"consecutive_failures"`
	NextRetryAt         *time.Time `db:"next_retry_at"`
	CurrentSnapshotID   string     `db:"current_snapshot_id"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
}
