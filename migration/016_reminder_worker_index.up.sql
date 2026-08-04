CREATE INDEX IF NOT EXISTS idx_users_active_reminders
    ON users(id)
    INCLUDE (default_group_id, reminder_minutes)
    WHERE reminder_enabled AND default_group_id IS NOT NULL;
