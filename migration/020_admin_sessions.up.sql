CREATE TABLE admin_sessions (
    token_hash  TEXT PRIMARY KEY,
    admin_id    TEXT NOT NULL,
    name        TEXT NOT NULL,
    auth_method TEXT NOT NULL CHECK (auth_method IN ('telegram', 'access_key')),
    admin_role  TEXT NOT NULL CHECK (admin_role IN ('read_only', 'support', 'editor', 'reviewer', 'operator', 'owner')),
    csrf_token  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_sessions_expiry ON admin_sessions(expires_at);
CREATE INDEX idx_admin_sessions_admin ON admin_sessions(admin_id);
