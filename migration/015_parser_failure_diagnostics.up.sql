ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;

UPDATE data_sources
SET next_retry_at = NOW() + (
    LEAST(
        21600.0,
        300.0 * POWER(
            2.0,
            LEAST(GREATEST(consecutive_failures - 1, 0), 7)
        )
    ) * INTERVAL '1 second'
)
WHERE COALESCE(last_error, '') <> ''
  AND next_retry_at IS NULL;

CREATE INDEX idx_data_sources_next_retry
    ON data_sources(next_retry_at)
    WHERE next_retry_at IS NOT NULL;

CREATE TABLE parser_diagnostics (
    id                  TEXT PRIMARY KEY,
    parse_log_id        TEXT NOT NULL REFERENCES parse_logs(id) ON DELETE CASCADE,
    data_source_id      TEXT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    stage               TEXT NOT NULL,
    category            TEXT NOT NULL,
    summary             TEXT NOT NULL,
    group_id            TEXT NOT NULL DEFAULT '',
    http_status         INT NOT NULL DEFAULT 0
                        CHECK (http_status BETWEEN 0 AND 599),
    content_type        TEXT NOT NULL DEFAULT '',
    response_size       INT NOT NULL DEFAULT 0
                        CHECK (response_size >= 0),
    response_sha256     TEXT NOT NULL DEFAULT '',
    response_preview    TEXT NOT NULL DEFAULT '',
    occurrences         INT NOT NULL DEFAULT 1
                        CHECK (occurrences > 0),
    metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_parser_diagnostics_source_created
    ON parser_diagnostics(data_source_id, created_at DESC);

CREATE INDEX idx_parser_diagnostics_log
    ON parser_diagnostics(parse_log_id);
