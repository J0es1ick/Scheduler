package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func ApplyRuntimeGrants(
	ctx context.Context,
	db *sqlx.DB,
	botRole string,
	adminRole string,
	parserRole string,
	privacyRole string,
) error {
	roles := []string{botRole, adminRole, parserRole, privacyRole}
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if role == "" {
			return fmt.Errorf("runtime roles must be nonempty and distinct")
		}
		if _, exists := seen[role]; exists {
			return fmt.Errorf("runtime roles must be nonempty and distinct")
		}
		seen[role] = struct{}{}
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var databaseName string
	if err = tx.GetContext(ctx, &databaseName, `SELECT current_database()`); err != nil {
		return err
	}
	for _, role := range roles {
		if _, err = tx.ExecContext(ctx, "REVOKE TEMPORARY ON DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" FROM "+pgx.Identifier{role}.Sanitize()); err != nil {
			return err
		}
		quoted := pgx.Identifier{role}.Sanitize()
		for _, statement := range []string{
			"REVOKE ALL ON ALL TABLES IN SCHEMA public FROM " + quoted,
			"REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM " + quoted,
			"REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM " + quoted,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM " + quoted,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM " + quoted,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM " + quoted,
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

	parserTables := "universities, groups, semesters, lessons, data_sources, parse_logs, parser_snapshots, parser_diagnostics, lesson_source_identities, group_source_identity_mappings, group_identity_conflicts, publication_reconciliation_queue, connector_ingestion_runs"
	readOnly := "schema_migrations, effective_lessons, public_site_statistics, public_site_universities, public_site_sources, subscription_integrity, notification_queue_eligibility"
	botRead := "universities, groups, semesters, data_sources, parse_logs, parser_snapshots, parser_diagnostics, lesson_source_identities, group_source_identity_mappings, group_identity_conflicts, publication_reconciliation_queue, connector_ingestion_runs, lesson_overrides, connector_clients, admin_audit_logs"
	grants := map[string][]string{
		botRole: {
			"SELECT ON " + readOnly,
			"SELECT ON " + botRead,
			"SELECT, INSERT, UPDATE, DELETE ON subscriptions, chat_schedule_profiles, notification_deliveries, bot_outbox, worker_status",
			"SELECT, INSERT ON support_requests",
			"SELECT, DELETE ON schedule_change_events",
			"SELECT ON users",
			"INSERT (id, username, created_at, updated_at) ON users",
			"UPDATE (username, default_group_id, notifications_enabled, reminder_enabled, reminder_minutes, quiet_hours_enabled, quiet_hours_start, quiet_hours_end, search_schedule_view_format, telegram_menu_fingerprint, updated_at) ON users",
			"SELECT (token_hash, admin_id, name, auth_method, admin_role, expires_at, created_at, last_seen_at) ON admin_sessions",
		},
		adminRole: {
			"SELECT, INSERT, UPDATE, DELETE ON " + parserTables,
			"SELECT ON " + readOnly,
			"SELECT, INSERT, UPDATE, DELETE ON lesson_overrides, connector_clients, connector_request_nonces, admin_sessions, privacy_deletion_requests",
			"SELECT, UPDATE ON users, support_requests",
			"SELECT, DELETE ON subscriptions",
			"SELECT ON chat_schedule_profiles, worker_status",
			"SELECT, UPDATE ON operational_maintenance",
			"SELECT, INSERT ON schedule_change_events, notification_deliveries, bot_outbox",
			"SELECT, INSERT, DELETE ON admin_audit_logs",
		},
		parserRole: {
			"SELECT, INSERT, UPDATE, DELETE ON " + parserTables,
			"SELECT ON " + readOnly,
			"SELECT ON lesson_overrides",
			"SELECT, UPDATE ON operational_maintenance",
		},
		privacyRole: {
			"SELECT ON schema_migrations",
			"SELECT, UPDATE, DELETE ON privacy_deletion_requests",
		},
	}
	for role, statements := range grants {
		for _, grant := range statements {
			if _, err = tx.ExecContext(ctx, "GRANT "+grant+" TO "+pgx.Identifier{role}.Sanitize()); err != nil {
				return fmt.Errorf("grant runtime privileges: %w", err)
			}
		}
	}

	functions := map[string][]string{
		botRole: {
			"scheduler_request_notification_cancellation(TEXT, TEXT, TEXT)",
			"scheduler_request_outbox_cancellation(TEXT, TEXT, TEXT, TEXT)",
			"scheduler_reconcile_notification_queue()",
			"enqueue_privacy_deletion(TEXT)",
		},
		adminRole: {
			"scheduler_request_notification_cancellation(TEXT, TEXT, TEXT)",
			"scheduler_request_outbox_cancellation(TEXT, TEXT, TEXT, TEXT)",
			"scheduler_reconcile_notification_queue()",
			"enqueue_schedule_change(TEXT, TEXT, TEXT, TEXT)",
			"enqueue_admin_alert(TEXT, TEXT)",
		},
		parserRole: {
			"enqueue_schedule_change(TEXT, TEXT, TEXT, TEXT)",
			"enqueue_admin_alert(TEXT, TEXT)",
		},
		privacyRole: {
			"execute_privacy_deletion(TEXT, TEXT)",
		},
	}
	for role, signatures := range functions {
		quoted := pgx.Identifier{role}.Sanitize()
		for _, signature := range signatures {
			if _, err = tx.ExecContext(ctx, "GRANT EXECUTE ON FUNCTION "+signature+" TO "+quoted); err != nil {
				return fmt.Errorf("grant runtime function privileges: %w", err)
			}
		}
	}
	return tx.Commit()
}
