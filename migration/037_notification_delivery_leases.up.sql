ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS claim_token TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

ALTER TABLE bot_outbox
    ADD COLUMN IF NOT EXISTS claim_token TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_claim_lease
    ON notification_deliveries(lease_expires_at)
    WHERE status = 'pending' AND claim_token <> '';

CREATE INDEX IF NOT EXISTS idx_bot_outbox_claim_lease
    ON bot_outbox(lease_expires_at)
    WHERE status = 'pending' AND claim_token <> '';
