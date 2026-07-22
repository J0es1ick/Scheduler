-- The original schema stored Moscow wall-clock values in TIMESTAMP columns.
-- Convert existing values as Europe/Moscow and keep future instants unambiguous.
ALTER TABLE universities
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE groups
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE lessons
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE subscriptions
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE data_sources
    ALTER COLUMN last_run_at TYPE TIMESTAMPTZ USING last_run_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE parse_logs
    ALTER COLUMN started_at TYPE TIMESTAMPTZ USING started_at AT TIME ZONE 'Europe/Moscow',
    ALTER COLUMN finished_at TYPE TIMESTAMPTZ USING finished_at AT TIME ZONE 'Europe/Moscow';

ALTER TABLE admin_audit_logs
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Europe/Moscow';
