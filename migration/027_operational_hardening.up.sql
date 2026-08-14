ALTER TABLE connector_ingestion_runs
    ADD COLUMN claim_token TEXT NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at TIMESTAMPTZ;

UPDATE connector_ingestion_runs
SET lease_expires_at = claimed_at + INTERVAL '2 minutes'
WHERE status='processing' AND claimed_at IS NOT NULL;

CREATE INDEX idx_connector_ingestion_lease
    ON connector_ingestion_runs(lease_expires_at)
    WHERE status='processing';

CREATE TABLE lesson_source_identities (
    lesson_id      TEXT PRIMARY KEY,
    university_id  TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    semester_id    TEXT NOT NULL REFERENCES semesters(id) ON DELETE CASCADE,
    source_id      TEXT NOT NULL DEFAULT '',
    external_id    TEXT NOT NULL DEFAULT '',
    identity_key   TEXT NOT NULL,
    first_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lesson_source_identity_key
    ON lesson_source_identities(university_id, semester_id, identity_key);

CREATE UNIQUE INDEX idx_lesson_source_external_identity
    ON lesson_source_identities(source_id, semester_id, external_id)
    WHERE source_id<>'' AND external_id<>'';
