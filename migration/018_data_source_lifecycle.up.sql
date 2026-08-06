ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_data_sources_enabled
    ON data_sources(is_enabled);
