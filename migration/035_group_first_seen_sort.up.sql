CREATE INDEX idx_groups_university_active_created_at
    ON groups(university_id, is_active, created_at DESC);
