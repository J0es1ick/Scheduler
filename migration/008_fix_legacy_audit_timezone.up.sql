-- Audit rows created before migration 007 used PostgreSQL's UTC wall clock,
-- while migration 007 interpreted them as Europe/Moscow. Shift only those
-- legacy audit rows back to their actual local time.
UPDATE admin_audit_logs
SET created_at = created_at + INTERVAL '3 hours'
WHERE created_at < COALESCE(
    (
        SELECT applied_at AT TIME ZONE 'Europe/Moscow'
        FROM schema_migrations
        WHERE name = '007_timestamps_with_timezone.up.sql'
    ),
    '-infinity'::TIMESTAMPTZ
);
