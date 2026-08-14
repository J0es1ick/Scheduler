CREATE TABLE publication_reconciliation_queue (
    university_id TEXT PRIMARY KEY REFERENCES universities(id) ON DELETE CASCADE,
    snapshot_id   TEXT NOT NULL REFERENCES parser_snapshots(id) ON DELETE RESTRICT,
    reason        TEXT NOT NULL,
    attempts      INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    claim_token   TEXT NOT NULL DEFAULT '',
    claimed_at    TIMESTAMPTZ,
    last_error    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_publication_reconciliation_claim
    ON publication_reconciliation_queue(claimed_at, created_at);

CREATE TEMP TABLE migration_026_affected_publications ON COMMIT DROP AS
WITH migration_time AS (
    SELECT applied_at
    FROM schema_migrations
    WHERE name = '025_reconcile_inactive_publications.up.sql'
), ranked AS (
    SELECT
        source.id AS source_id,
        source.university_id,
        snapshot.id AS snapshot_id,
        source.last_success_at,
        ROW_NUMBER() OVER (
            PARTITION BY source.id
            ORDER BY snapshot.created_at DESC, snapshot.id
        ) AS position
    FROM data_sources source
    JOIN parser_snapshots snapshot ON snapshot.data_source_id = source.id
    CROSS JOIN migration_time
    WHERE source.lifecycle_status <> 'active'
      AND source.current_snapshot_id IS NULL
      AND snapshot.status = 'approved'
      AND snapshot.published_at IS NULL
      AND snapshot.created_at <= migration_time.applied_at
)
SELECT source_id, university_id, snapshot_id, last_success_at
FROM ranked
WHERE position = 1;

UPDATE parser_snapshots snapshot
SET status = 'published',
    published_at = COALESCE(
        affected.last_success_at,
        snapshot.reviewed_at,
        snapshot.created_at
    )
FROM migration_026_affected_publications affected
WHERE snapshot.id = affected.snapshot_id
  AND NOT EXISTS (
      SELECT 1
      FROM data_sources active_source
      WHERE active_source.university_id = affected.university_id
        AND active_source.lifecycle_status = 'active'
  );

UPDATE data_sources source
SET current_snapshot_id = affected.snapshot_id
FROM migration_026_affected_publications affected
WHERE source.id = affected.source_id
  AND NOT EXISTS (
      SELECT 1
      FROM data_sources active_source
      WHERE active_source.university_id = affected.university_id
        AND active_source.lifecycle_status = 'active'
  );

UPDATE connector_ingestion_runs run
SET status = 'published'
FROM migration_026_affected_publications affected
WHERE run.parser_snapshot_id = affected.snapshot_id
  AND run.status = 'staged'
  AND NOT EXISTS (
      SELECT 1
      FROM data_sources active_source
      WHERE active_source.university_id = affected.university_id
        AND active_source.lifecycle_status = 'active'
  );

SELECT pg_advisory_xact_lock(
    hashtext('scheduler-snapshot-publication'),
    hashtext(affected.university_id)
)
FROM (
    SELECT DISTINCT affected.university_id
    FROM migration_026_affected_publications affected
    JOIN data_sources active_source
      ON active_source.university_id = affected.university_id
     AND active_source.lifecycle_status = 'active'
) affected;

INSERT INTO publication_reconciliation_queue (
    university_id,
    snapshot_id,
    reason
)
SELECT DISTINCT ON (active_source.university_id)
    active_source.university_id,
    active_snapshot.id,
    'restore active source after migration 025'
FROM migration_026_affected_publications affected
JOIN data_sources active_source
  ON active_source.university_id = affected.university_id
 AND active_source.lifecycle_status = 'active'
JOIN parser_snapshots active_snapshot
  ON active_snapshot.id = active_source.current_snapshot_id
 AND active_snapshot.status = 'published'
ORDER BY active_source.university_id, active_snapshot.created_at DESC
ON CONFLICT (university_id) DO UPDATE SET
    snapshot_id = EXCLUDED.snapshot_id,
    reason = EXCLUDED.reason,
    attempts = 0,
    claim_token = '',
    claimed_at = NULL,
    last_error = '';
