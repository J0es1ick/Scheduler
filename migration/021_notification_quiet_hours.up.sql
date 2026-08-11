ALTER TABLE users
    ADD COLUMN IF NOT EXISTS quiet_hours_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS quiet_hours_start TIME NOT NULL DEFAULT '22:00',
    ADD COLUMN IF NOT EXISTS quiet_hours_end TIME NOT NULL DEFAULT '07:00';

ALTER TABLE users
    ADD CONSTRAINT users_quiet_hours_range_check
        CHECK (quiet_hours_start<>quiet_hours_end);
