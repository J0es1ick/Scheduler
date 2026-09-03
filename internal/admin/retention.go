package admin

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/database"
)

func (s *Store) RunRetention(ctx context.Context) (bool, error) {
	return database.RunRetention(ctx, s.db, "admin_retention", []string{
		`DELETE FROM admin_audit_logs WHERE ctid IN (
			SELECT ctid FROM admin_audit_logs WHERE created_at<NOW()-INTERVAL '365 days' LIMIT 1000
		)`,
		`DELETE FROM connector_request_nonces WHERE ctid IN (
			SELECT ctid FROM connector_request_nonces WHERE expires_at<NOW() LIMIT 1000
		)`,
		`DELETE FROM admin_sessions WHERE token_hash IN (
			SELECT token_hash FROM admin_sessions WHERE expires_at<NOW() LIMIT 1000
		)`,
	})
}
