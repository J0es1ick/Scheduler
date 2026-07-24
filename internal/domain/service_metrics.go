package domain

import "time"

type ServiceMetrics struct {
	Universities          int `db:"universities"`
	Groups                int `db:"groups"`
	Lessons               int `db:"lessons"`
	Users                 int `db:"users"`
	Subscriptions         int `db:"subscriptions"`
	SourcesTotal          int
	SourcesHealthy        int
	SourcesRunning        int
	SourcesStale          int
	SourcesError          int
	SourcesQuarantined    int
	PendingNotifications  int        `db:"pending_notifications"`
	FailedNotifications   int        `db:"failed_notifications"`
	PendingOutbox         int        `db:"pending_outbox"`
	FailedOutbox          int        `db:"failed_outbox"`
	LastSuccessfulParseAt *time.Time `db:"last_successful_parse_at"`
	CheckedAt             time.Time
}
