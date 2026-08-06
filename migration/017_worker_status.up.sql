CREATE TABLE worker_status (
    name                  TEXT PRIMARY KEY,
    cursor                TEXT NOT NULL DEFAULT '',
    last_started_at       TIMESTAMPTZ,
    last_finished_at      TIMESTAMPTZ,
    last_full_cycle_at    TIMESTAMPTZ,
    last_duration_ms      BIGINT NOT NULL DEFAULT 0 CHECK (last_duration_ms >= 0),
    last_processed        INT NOT NULL DEFAULT 0 CHECK (last_processed >= 0),
    last_failures         INT NOT NULL DEFAULT 0 CHECK (last_failures >= 0),
    last_error            TEXT NOT NULL DEFAULT '',
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO worker_status (name)
VALUES ('lesson_reminders')
ON CONFLICT (name) DO NOTHING;
