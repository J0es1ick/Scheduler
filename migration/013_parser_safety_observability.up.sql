DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'parse_logs'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) ILIKE '%status%'
    LOOP
        EXECUTE format('ALTER TABLE parse_logs DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;

ALTER TABLE parse_logs
    ALTER COLUMN status TYPE TEXT USING status::TEXT;

DROP TYPE parse_log_status;

ALTER TABLE parse_logs
    ADD CONSTRAINT parse_logs_status_lifecycle_check CHECK (
        status IN ('running', 'success', 'failed', 'quarantined')
        AND (
            (status = 'running' AND finished_at IS NULL)
            OR (status <> 'running' AND finished_at IS NOT NULL)
        )
    );

CREATE TABLE parser_snapshots (
    id                TEXT PRIMARY KEY,
    data_source_id    TEXT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    parse_log_id      TEXT NOT NULL REFERENCES parse_logs(id) ON DELETE CASCADE,
    status            TEXT NOT NULL
                      CHECK (status IN ('staged', 'quarantined', 'published', 'rejected')),
    publishable       BOOLEAN NOT NULL DEFAULT TRUE,
    group_count       INT NOT NULL CHECK (group_count >= 0),
    lesson_count      INT NOT NULL CHECK (lesson_count >= 0),
    anomaly_reasons   JSONB NOT NULL DEFAULT '[]'::jsonb,
    payload           JSONB NOT NULL,
    reviewed_by       TEXT NOT NULL DEFAULT '',
    review_note       TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at      TIMESTAMPTZ,
    reviewed_at       TIMESTAMPTZ
);

CREATE INDEX idx_parser_snapshots_source_created
    ON parser_snapshots(data_source_id, created_at DESC);

CREATE INDEX idx_parser_snapshots_quarantine
    ON parser_snapshots(status, created_at DESC)
    WHERE status = 'quarantined';

ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS last_success_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS consecutive_failures INT NOT NULL DEFAULT 0
        CHECK (consecutive_failures >= 0),
    ADD COLUMN IF NOT EXISTS current_snapshot_id TEXT
        REFERENCES parser_snapshots(id) ON DELETE SET NULL;

UPDATE data_sources
SET last_success_at = last_run_at
WHERE last_success_at IS NULL
  AND COALESCE(last_error, '') = ''
  AND last_run_at IS NOT NULL;

ALTER TABLE bot_outbox
    DROP CONSTRAINT IF EXISTS bot_outbox_kind_check;

ALTER TABLE bot_outbox
    ADD CONSTRAINT bot_outbox_kind_check
    CHECK (kind IN ('support_request', 'support_resolution', 'admin_alert'));
