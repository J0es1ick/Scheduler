CREATE FUNCTION scheduler_lock_privacy_deletion_request(request_id TEXT, request_claim_token TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    deletion_user_id TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock_shared(hashtext('scheduler-group-references'));

    SELECT user_id INTO deletion_user_id
    FROM privacy_deletion_requests
    WHERE id=request_id AND status='processing' AND claim_token=request_claim_token
      AND lease_expires_at>clock_timestamp();
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    PERFORM 1 FROM users WHERE id=deletion_user_id FOR UPDATE;

    SELECT user_id INTO deletion_user_id
    FROM privacy_deletion_requests
    WHERE id=request_id AND user_id=deletion_user_id
      AND status='processing' AND claim_token=request_claim_token
      AND lease_expires_at>clock_timestamp()
    FOR UPDATE;
    RETURN deletion_user_id;
END;
$$;

REVOKE ALL ON FUNCTION scheduler_lock_privacy_deletion_request(TEXT, TEXT) FROM PUBLIC;

DO $$
BEGIN
    EXECUTE format('ALTER FUNCTION scheduler_lock_privacy_deletion_request(TEXT, TEXT) SET search_path TO pg_catalog, %I, pg_temp', current_schema());
END;
$$;
