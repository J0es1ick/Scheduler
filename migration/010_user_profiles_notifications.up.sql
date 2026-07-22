ALTER TABLE users
    ADD COLUMN IF NOT EXISTS default_group_id TEXT REFERENCES groups(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE users u
SET default_group_id = (
    SELECT s.object_id
    FROM subscriptions s
    JOIN groups g ON g.id = s.object_id AND g.is_active
    WHERE s.user_id = u.id AND s.object_type = 'group'
    ORDER BY s.updated_at DESC, s.created_at DESC
    LIMIT 1
)
WHERE u.default_group_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM subscriptions s
      JOIN groups g ON g.id = s.object_id AND g.is_active
      WHERE s.user_id = u.id AND s.object_type = 'group'
  );

CREATE INDEX IF NOT EXISTS idx_users_default_group_id
    ON users(default_group_id);

CREATE TABLE schedule_change_events (
    id         TEXT PRIMARY KEY,
    group_id   TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    source     TEXT NOT NULL CHECK (source IN ('parser', 'manual')),
    summary    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_deliveries (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES schedule_change_events(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    attempts        INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(event_id, user_id)
);

CREATE INDEX idx_notification_deliveries_pending
    ON notification_deliveries(status, next_attempt_at);

CREATE INDEX idx_schedule_change_events_created_at
    ON schedule_change_events(created_at DESC);

CREATE OR REPLACE FUNCTION enqueue_schedule_change(
    event_id TEXT,
    changed_group_id TEXT,
    change_source TEXT,
    change_summary TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO schedule_change_events (id, group_id, source, summary)
    VALUES (event_id, changed_group_id, change_source, change_summary);

    INSERT INTO notification_deliveries (id, event_id, user_id)
    SELECT event_id || ':' || s.user_id, event_id, s.user_id
    FROM subscriptions s
    JOIN users u ON u.id = s.user_id
    WHERE s.object_type = 'group'
      AND s.object_id = changed_group_id
      AND u.notifications_enabled
    ON CONFLICT ON CONSTRAINT notification_deliveries_event_id_user_id_key DO NOTHING;
END;
$$ LANGUAGE plpgsql;
