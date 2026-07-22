CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id          TEXT PRIMARY KEY,
    actor_id    TEXT NOT NULL,
    actor_name  TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id   TEXT NOT NULL DEFAULT '',
    details     JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_created_at
    ON admin_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_actor
    ON admin_audit_logs(actor_id, created_at DESC);
