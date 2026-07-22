CREATE TABLE support_requests (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_type TEXT NOT NULL CHECK (request_type IN ('update_existing', 'new_institution')),
    details     TEXT NOT NULL CHECK (char_length(details) BETWEEN 20 AND 4096),
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'approved', 'rejected')),
    review_note TEXT NOT NULL DEFAULT '',
    reviewed_by TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_support_requests_status_created
    ON support_requests(status, created_at DESC);

CREATE INDEX idx_support_requests_user
    ON support_requests(user_id, created_at DESC);

CREATE TABLE bot_outbox (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_id      TEXT REFERENCES support_requests(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('support_request', 'support_resolution')),
    body            TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4096),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    attempts        INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bot_outbox_pending
    ON bot_outbox(status, next_attempt_at);
