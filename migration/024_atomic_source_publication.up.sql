ALTER TABLE parser_snapshots
    DROP CONSTRAINT IF EXISTS parser_snapshots_status_check;

ALTER TABLE parser_snapshots
    ADD CONSTRAINT parser_snapshots_status_check
    CHECK (status IN ('staged', 'quarantined', 'approved', 'published', 'rejected'));

CREATE INDEX IF NOT EXISTS idx_parser_snapshots_approved
    ON parser_snapshots(data_source_id, created_at DESC)
    WHERE status = 'approved';
