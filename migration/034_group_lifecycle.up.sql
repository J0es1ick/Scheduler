ALTER TABLE groups
    ADD COLUMN source_active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN manually_disabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE groups
SET source_active = is_active;

CREATE INDEX idx_groups_university_lifecycle
    ON groups(university_id, is_active, source_active, manually_disabled);
