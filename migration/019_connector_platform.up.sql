ALTER TABLE universities
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Europe/Moscow',
    ADD COLUMN IF NOT EXISTS locale TEXT NOT NULL DEFAULT 'ru-RU',
    ADD COLUMN IF NOT EXISTS first_weekday SMALLINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS academic_week_anchor DATE;

ALTER TABLE universities
    ADD CONSTRAINT universities_first_weekday_check
        CHECK (first_weekday BETWEEN 1 AND 7);

ALTER TABLE semesters
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS academic_year TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE semesters
SET external_id = id
WHERE external_id IS NULL OR external_id = '';

ALTER TABLE semesters
    ALTER COLUMN external_id SET NOT NULL,
    ADD CONSTRAINT semesters_status_check
        CHECK (status IN ('draft', 'active', 'archived'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_semesters_university_external
    ON semesters(university_id, external_id);

ALTER TABLE lessons
    ADD COLUMN IF NOT EXISTS recurrence JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS source_id TEXT,
    ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fetched_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_fingerprint TEXT NOT NULL DEFAULT '';

ALTER TABLE lesson_overrides
    ADD COLUMN IF NOT EXISTS recurrence JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lessons
    ADD CONSTRAINT lessons_time_format_check
        CHECK (
            time_start ~ '^(?:[01][0-9]|2[0-3]):[0-5][0-9]$'
            AND time_end ~ '^(?:[01][0-9]|2[0-3]):[0-5][0-9]$'
            AND time_start::time < time_end::time
        ) NOT VALID;

ALTER TABLE lesson_overrides
    ADD CONSTRAINT lesson_overrides_time_format_check
        CHECK (
            time_start ~ '^(?:[01][0-9]|2[0-3]):[0-5][0-9]$'
            AND time_end ~ '^(?:[01][0-9]|2[0-3]):[0-5][0-9]$'
            AND time_start::time < time_end::time
        ) NOT VALID;

DROP VIEW effective_lessons;

CREATE VIEW effective_lessons AS
SELECT
    l.id,
    l.university_id,
    l.semester_id,
    l.day_of_week,
    l.special_date,
    l.time_start,
    l.time_end,
    l.week_type,
    l.subject,
    l.type,
    l.teacher,
    l.room,
    l.group_id,
    l.subgroup,
    l.valid_from,
    l.valid_to,
    l.recurrence,
    l.source_id,
    l.external_id,
    l.fetched_at,
    l.source_fingerprint,
    l.updated_at,
    'parsed'::TEXT AS origin,
    NULL::TEXT AS base_lesson_id,
    0::BIGINT AS version
FROM lessons l
WHERE NOT EXISTS (
    SELECT 1
    FROM lesson_overrides o
    WHERE o.base_lesson_id = l.id
)
UNION ALL
SELECT
    o.id,
    o.university_id,
    o.semester_id,
    o.day_of_week,
    o.special_date,
    o.time_start,
    o.time_end,
    o.week_type,
    o.subject,
    o.type,
    o.teacher,
    o.room,
    o.group_id,
    o.subgroup,
    o.valid_from,
    o.valid_to,
    o.recurrence,
    NULL::TEXT AS source_id,
    ''::TEXT AS external_id,
    NULL::TIMESTAMPTZ AS fetched_at,
    ''::TEXT AS source_fingerprint,
    o.updated_at,
    'manual'::TEXT AS origin,
    o.base_lesson_id,
    o.version
FROM lesson_overrides o
WHERE NOT o.is_deleted;

ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS quality_policy JSONB NOT NULL DEFAULT '{
        "allow_empty": false,
        "minimum_groups": 1,
        "minimum_lessons": 0,
        "maximum_group_drop_ratio": 0.30,
        "maximum_group_growth_ratio": 0.80,
        "maximum_lesson_drop_ratio": 0.40,
        "maximum_lesson_growth_ratio": 1.00
    }'::jsonb;

ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_lifecycle_status_check
        CHECK (lifecycle_status IN ('draft', 'testing', 'pending_review', 'active', 'suspended', 'archived'));

CREATE TABLE connector_clients (
    id                    TEXT PRIMARY KEY,
    data_source_id        TEXT NOT NULL UNIQUE REFERENCES data_sources(id) ON DELETE RESTRICT,
    display_name          TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    maintainer_name       TEXT NOT NULL DEFAULT '',
    maintainer_url        TEXT NOT NULL DEFAULT '',
    key_id                TEXT NOT NULL UNIQUE,
    public_key            BYTEA NOT NULL,
    status                TEXT NOT NULL DEFAULT 'draft'
                          CHECK (status IN ('draft', 'testing', 'pending_review', 'active', 'suspended', 'archived')),
    rate_limit_per_minute INT NOT NULL DEFAULT 30 CHECK (rate_limit_per_minute BETWEEN 1 AND 600),
    max_payload_bytes     INT NOT NULL DEFAULT 16777216 CHECK (max_payload_bytes BETWEEN 1024 AND 67108864),
    last_seen_at          TIMESTAMPTZ,
    last_snapshot_at      TIMESTAMPTZ,
    created_by            TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_clients_status
    ON connector_clients(status);

CREATE TABLE connector_ingestion_runs (
    id                   TEXT PRIMARY KEY,
    connector_id         TEXT NOT NULL REFERENCES connector_clients(id) ON DELETE RESTRICT,
    external_snapshot_id TEXT NOT NULL,
    schema_version       TEXT NOT NULL,
    idempotency_key      TEXT NOT NULL,
    payload_sha256       TEXT NOT NULL,
    payload              JSONB NOT NULL,
    status               TEXT NOT NULL DEFAULT 'received'
                         CHECK (status IN (
                            'received', 'processing', 'staged', 'quarantined',
                            'published', 'rejected', 'failed'
                         )),
    attempts             INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error_message        TEXT NOT NULL DEFAULT '',
    parser_snapshot_id   TEXT REFERENCES parser_snapshots(id) ON DELETE SET NULL,
    group_count          INT NOT NULL DEFAULT 0 CHECK (group_count >= 0),
    lesson_count         INT NOT NULL DEFAULT 0 CHECK (lesson_count >= 0),
    next_attempt_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at           TIMESTAMPTZ,
    received_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at         TIMESTAMPTZ,
    UNIQUE(connector_id, external_snapshot_id),
    UNIQUE(connector_id, idempotency_key)
);

CREATE INDEX idx_connector_ingestion_runs_pending
    ON connector_ingestion_runs(status, next_attempt_at, received_at)
    WHERE status IN ('received', 'processing');

CREATE INDEX idx_connector_ingestion_runs_connector
    ON connector_ingestion_runs(connector_id, received_at DESC);

CREATE TABLE connector_request_nonces (
    connector_id TEXT NOT NULL REFERENCES connector_clients(id) ON DELETE CASCADE,
    nonce         TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (connector_id, nonce)
);

CREATE INDEX idx_connector_request_nonces_expiry
    ON connector_request_nonces(expires_at);

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS admin_role TEXT NOT NULL DEFAULT 'none';

UPDATE users
SET admin_role = 'owner'
WHERE is_admin AND admin_role = 'none';

ALTER TABLE users
    ADD CONSTRAINT users_admin_role_check
        CHECK (admin_role IN ('none', 'read_only', 'support', 'editor', 'reviewer', 'operator', 'owner'));
