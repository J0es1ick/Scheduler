CREATE OR REPLACE FUNCTION execute_privacy_deletion(deletion_user_id TEXT, anonymized_user_id TEXT)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    profile_is_admin BOOLEAN;
    profile_admin_role TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock_shared(hashtext('scheduler-group-references'));

    SELECT is_admin, admin_role
    INTO profile_is_admin, profile_admin_role
    FROM users
    WHERE id=deletion_user_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    IF profile_is_admin OR profile_admin_role<>'none' THEN
        RAISE EXCEPTION 'remove administrator role before deleting the profile';
    END IF;

    DELETE FROM admin_sessions WHERE admin_id=deletion_user_id;
    UPDATE admin_audit_logs
    SET actor_id=anonymized_user_id, actor_name='Удалённый пользователь', ip_address=''
    WHERE actor_id=deletion_user_id;
    UPDATE chat_schedule_profiles SET configured_by=anonymized_user_id WHERE configured_by=deletion_user_id;
    UPDATE lesson_overrides SET created_by=anonymized_user_id WHERE created_by=deletion_user_id;
    UPDATE support_requests SET reviewed_by=anonymized_user_id WHERE reviewed_by=deletion_user_id;
    UPDATE parser_snapshots SET reviewed_by=anonymized_user_id WHERE reviewed_by=deletion_user_id;
    UPDATE connector_clients SET created_by=anonymized_user_id WHERE created_by=deletion_user_id;
    DELETE FROM users WHERE id=deletion_user_id;
    RETURN TRUE;
END;
$$;

REVOKE ALL ON FUNCTION execute_privacy_deletion(TEXT, TEXT) FROM PUBLIC;
