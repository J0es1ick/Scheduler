ALTER TABLE users
    ADD COLUMN search_schedule_view_format TEXT NOT NULL DEFAULT 'visual';

ALTER TABLE users
    ADD CONSTRAINT users_search_schedule_view_format_check
    CHECK (search_schedule_view_format IN ('compact', 'visual'));
