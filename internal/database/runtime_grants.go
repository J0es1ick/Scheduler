package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func ApplyRuntimeGrants(ctx context.Context, db *sqlx.DB, botRole, adminRole string) error {
	if botRole == "" || adminRole == "" || botRole == adminRole {
		return fmt.Errorf("runtime roles must be nonempty and distinct")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, role := range []string{botRole, adminRole} {
		quoted := pgx.Identifier{role}.Sanitize()
		for _, statement := range []string{
			"REVOKE ALL ON ALL TABLES IN SCHEMA public FROM " + quoted,
			"REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM " + quoted,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM " + quoted,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM " + quoted,
			"GRANT USAGE ON SCHEMA public TO " + quoted,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("reset runtime privileges: %w", err)
			}
		}
		var columns []struct {
			Table  string `db:"table_name"`
			Column string `db:"column_name"`
		}
		if err = tx.SelectContext(ctx, &columns, `
			SELECT c.relname AS table_name, a.attname AS column_name
			FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname='public' AND a.attnum>0 AND NOT a.attisdropped AND a.attacl IS NOT NULL`); err != nil {
			return err
		}
		for _, column := range columns {
			statement := "REVOKE ALL (" + pgx.Identifier{column.Column}.Sanitize() + ") ON TABLE " + pgx.Identifier{"public", column.Table}.Sanitize() + " FROM " + quoted
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("reset runtime column privileges: %w", err)
			}
		}
	}
	common := "universities, groups, semesters, lessons, data_sources, parse_logs, parser_snapshots, parser_diagnostics, lesson_source_identities, group_source_identity_mappings, group_identity_conflicts, publication_reconciliation_queue, connector_ingestion_runs"
	readOnly := "schema_migrations, effective_lessons, public_site_statistics, public_site_universities, public_site_sources, subscription_integrity"
	grants := map[string][]string{
		botRole: {
			"SELECT, INSERT, UPDATE, DELETE ON " + common,
			"SELECT ON " + readOnly,
			"SELECT, INSERT, UPDATE, DELETE ON subscriptions, chat_schedule_profiles, support_requests, schedule_change_events, notification_deliveries, bot_outbox, worker_status, operational_maintenance",
			"SELECT, DELETE ON users",
			"INSERT (id, username, created_at, updated_at) ON users",
			"UPDATE (username, default_group_id, notifications_enabled, reminder_enabled, reminder_minutes, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, search_schedule_view_format, telegram_menu_fingerprint, updated_at) ON users",
			"SELECT, UPDATE ON connector_clients",
			"SELECT, DELETE ON connector_request_nonces",
			"SELECT ON lesson_overrides, admin_audit_logs",
			"DELETE ON admin_audit_logs",
			"UPDATE (created_by) ON lesson_overrides",
			"UPDATE (actor_id, actor_name, ip_address) ON admin_audit_logs",
			"SELECT (token_hash, admin_id, name, auth_method, admin_role, expires_at, created_at, last_seen_at) ON admin_sessions",
			"DELETE ON admin_sessions",
		},
		adminRole: {
			"SELECT, INSERT, UPDATE, DELETE ON " + common,
			"SELECT ON " + readOnly,
			"SELECT, INSERT, UPDATE, DELETE ON lesson_overrides, connector_clients, connector_request_nonces, admin_sessions",
			"SELECT, UPDATE ON users, support_requests",
			"SELECT, DELETE ON subscriptions",
			"SELECT ON chat_schedule_profiles, worker_status, operational_maintenance",
			"SELECT, INSERT ON schedule_change_events, notification_deliveries, bot_outbox, admin_audit_logs",
		},
	}
	for role, statements := range grants {
		for _, grant := range statements {
			if _, err = tx.ExecContext(ctx, "GRANT "+grant+" TO "+pgx.Identifier{role}.Sanitize()); err != nil {
				return fmt.Errorf("grant runtime privileges: %w", err)
			}
		}
	}
	return tx.Commit()
}
