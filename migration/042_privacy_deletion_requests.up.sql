CREATE TABLE privacy_deletion_requests (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_token TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT privacy_deletion_requests_status_check CHECK (status IN ('pending', 'processing')),
    CONSTRAINT privacy_deletion_requests_attempts_check CHECK (attempts >= 0),
    CONSTRAINT privacy_deletion_requests_claim_check CHECK (
        (status = 'pending' AND claim_token = '' AND lease_expires_at IS NULL)
        OR
        (status = 'processing' AND claim_token <> '' AND lease_expires_at IS NOT NULL)
    )
);

CREATE INDEX idx_privacy_deletion_requests_pending
    ON privacy_deletion_requests (next_attempt_at, requested_at, id)
    WHERE status = 'pending';

CREATE INDEX idx_privacy_deletion_requests_expired
    ON privacy_deletion_requests (lease_expires_at, requested_at, id)
    WHERE status = 'processing';

CREATE OR REPLACE FUNCTION enqueue_privacy_deletion(deletion_user_id TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    request_id TEXT;
BEGIN
    PERFORM 1
    FROM users
    WHERE id = deletion_user_id AND NOT is_admin AND admin_role = 'none'
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'user is absent or has an administrator role';
    END IF;

    request_id := gen_random_uuid()::TEXT;
    INSERT INTO privacy_deletion_requests (id, user_id)
    VALUES (request_id, deletion_user_id)
    ON CONFLICT (user_id) DO UPDATE SET user_id=EXCLUDED.user_id
    RETURNING id INTO request_id;
    RETURN request_id;
END;
$$;

REVOKE ALL ON FUNCTION enqueue_privacy_deletion(TEXT) FROM PUBLIC;
