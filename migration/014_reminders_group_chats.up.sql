ALTER TABLE users
    ADD COLUMN IF NOT EXISTS reminder_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS reminder_minutes INT NOT NULL DEFAULT 15;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_reminder_minutes_check;

ALTER TABLE users
    ADD CONSTRAINT users_reminder_minutes_check
    CHECK (reminder_minutes BETWEEN 5 AND 180);

CREATE TABLE chat_schedule_profiles (
    chat_id          TEXT PRIMARY KEY,
    title            TEXT NOT NULL DEFAULT '',
    default_group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    configured_by    TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_schedule_profiles_group
    ON chat_schedule_profiles(default_group_id);

ALTER TABLE bot_outbox
    ADD COLUMN IF NOT EXISTS group_id TEXT REFERENCES groups(id) ON DELETE CASCADE;

ALTER TABLE bot_outbox
    DROP CONSTRAINT IF EXISTS bot_outbox_kind_check;

ALTER TABLE bot_outbox
    ADD CONSTRAINT bot_outbox_kind_check
    CHECK (kind IN (
        'support_request',
        'support_resolution',
        'admin_alert',
        'lesson_reminder'
    ));

CREATE INDEX idx_bot_outbox_reminder_pending
    ON bot_outbox(user_id, group_id, status, next_attempt_at)
    WHERE kind = 'lesson_reminder';
